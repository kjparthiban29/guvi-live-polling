
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

type CreatePollRequest struct {
	Question string   `json:"question" binding:"required"`
	Options  []string `json:"options" binding:"required,min=2"`
}

type AuthRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type VoteRequest struct {
	OptionID int `json:"optionId" binding:"required"`
}

type User struct {
	ID           bson.ObjectID `bson:"_id,omitempty" json:"id"`
	Email        string        `bson:"email" json:"email"`
	PasswordHash string        `bson:"passwordHash" json:"-"`
	CreatedAt    time.Time     `bson:"createdAt" json:"createdAt"`
}

type Poll struct {
	ID        bson.ObjectID `bson:"_id,omitempty" json:"id"`
	OwnerID   bson.ObjectID `bson:"ownerId" json:"ownerId"`
	Question  string        `bson:"question" json:"question"`
	Options   []PollOption  `bson:"options" json:"options"`
	CreatedAt time.Time     `bson:"createdAt" json:"createdAt"`
}

type PollOption struct {
	ID   int    `bson:"id" json:"id"`
	Text string `bson:"text" json:"text"`
}

func makeToken(userID bson.ObjectID) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		return "", http.ErrNotSupported
	}

	claims := jwt.MapClaims{
		"sub": userID.Hex(),
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Please log in first."})
			return
		}

		secret := os.Getenv("JWT_SECRET")
		if len(secret) < 32 {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Authentication is not configured."})
			return
		}

		token, err := jwt.Parse(strings.TrimPrefix(header, "Bearer "),
			func(token *jwt.Token) (any, error) {
				if token.Method != jwt.SigningMethodHS256 {
					return nil, http.ErrNotSupported
				}
				return []byte(secret), nil
			})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired login token."})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token."})
			return
		}
		userID, ok := claims["sub"].(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token."})
			return
		}
		c.Set("userID", userID)
		c.Next()
	}
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoURI := os.Getenv("MONGODB_URI")
if mongoURI == "" {
    mongoURI = "mongodb://localhost:27017"
}

mongoClient, err := mongo.Connect(
    options.Client().ApplyURI(mongoURI),
)
	if err != nil {
		log.Fatal("MongoDB connection failed: ", err)
	}
	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Fatal("MongoDB ping failed: ", err)
	}
	defer mongoClient.Disconnect(context.Background())
	log.Println("MongoDB connected successfully")

	redisURL := os.Getenv("REDIS_URL")

var redisClient *redis.Client

if redisURL != "" {
    redisOptions, err := redis.ParseURL(redisURL)
    if err != nil {
        log.Fatal("Invalid Redis URL: ", err)
    }
    redisClient = redis.NewClient(redisOptions)
} else {
    redisAddr := os.Getenv("REDIS_ADDR")
    if redisAddr == "" {
        redisAddr = "localhost:6379"
    }
    redisClient = redis.NewClient(&redis.Options{Addr: redisAddr})
}
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal("Redis connection failed: ", err)
	}
	defer redisClient.Close()
	log.Println("Redis connected successfully")

	db := mongoClient.Database("quickpoll")
	users := db.Collection("users")
	polls := db.Collection("polls")

	// Prevent duplicate accounts with the same email.
	if _, err := users.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		log.Fatal("Could not create email index: ", err)
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{
    "http://localhost:5173",
    "http://localhost:5174",
},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
	}))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "QuickPoll backend is running!"})
	})

	router.POST("/api/auth/signup", func(c *gin.Context) {
		var request AuthRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Provide a valid email and password (at least 8 characters)."})
			return
		}

		request.Email = strings.ToLower(strings.TrimSpace(request.Email))
		if len(request.Password) > 72 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at most 72 bytes."})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not secure password."})
			return
		}

		user := User{
			ID: bson.NewObjectID(), Email: request.Email,
			PasswordHash: string(hash), CreatedAt: time.Now().UTC(),
		}
		_, err = users.InsertOne(c.Request.Context(), user)
		if err != nil {
			if mongo.IsDuplicateKeyError(err) {
				c.JSON(http.StatusConflict, gin.H{"error": "An account with this email already exists."})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create account."})
			return
		}

		token, err := makeToken(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Set JWT_SECRET to a random secret of at least 32 characters."})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"message": "Account created.", "token": token, "email": user.Email})
	})

	router.POST("/api/auth/login", func(c *gin.Context) {
		var request AuthRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Provide a valid email and password."})
			return
		}
		email := strings.ToLower(strings.TrimSpace(request.Email))

		var user User
		if err := users.FindOne(c.Request.Context(), bson.M{"email": email}).Decode(&user); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password."})
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(request.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password."})
			return
		}

		token, err := makeToken(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Set JWT_SECRET to a random secret of at least 32 characters."})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Login successful.", "token": token, "email": user.Email})
	})

	router.POST("/api/polls", authMiddleware(), func(c *gin.Context) {
		var request CreatePollRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a question and at least two options."})
			return
		}
		request.Question = strings.TrimSpace(request.Question)
		if request.Question == "" || len(request.Question) > 200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Question must be between 1 and 200 characters."})
			return
		}
		if len(request.Options) < 2 || len(request.Options) > 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Provide between 2 and 10 options."})
			return
		}

		ownerHex, _ := c.Get("userID")
		ownerID, err := bson.ObjectIDFromHex(ownerHex.(string))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid account."})
			return
		}

		pollOptions := make([]PollOption, 0, len(request.Options))
		for _, option := range request.Options {
			option = strings.TrimSpace(option)
			if option == "" || len(option) > 100 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Each option must be between 1 and 100 characters."})
				return
			}
			pollOptions = append(pollOptions, PollOption{ID: len(pollOptions) + 1, Text: option})
		}

		poll := Poll{
			ID: bson.NewObjectID(), OwnerID: ownerID,
			Question: request.Question, Options: pollOptions,
			CreatedAt: time.Now().UTC(),
		}
		insertCtx, insertCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer insertCancel()

		if _, err := polls.InsertOne(insertCtx, poll); err != nil {
			log.Println("Failed to save poll:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save the poll."})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"message": "Poll created successfully!", "poll": poll})
	})

	router.GET("/api/polls/:id", func(c *gin.Context) {
		id, err := bson.ObjectIDFromHex(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid poll ID."})
			return
		}
		var poll Poll
		if err := polls.FindOne(c.Request.Context(), bson.M{"_id": id}).Decode(&poll); err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Poll not found."})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve poll."})
			return
		}
		c.JSON(http.StatusOK, gin.H{"poll": poll})
	})

	router.POST("/api/polls/:id/vote", func(c *gin.Context) {
		pollID, err := bson.ObjectIDFromHex(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid poll ID."})
			return
		}
		var request VoteRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Provide a valid optionId."})
			return
		}

		var poll Poll
		if err := polls.FindOne(c.Request.Context(), bson.M{"_id": pollID}).Decode(&poll); err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Poll not found."})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve poll."})
			return
		}

		valid := false
		for _, option := range poll.Options {
			if option.ID == request.OptionID {
				valid = true
				break
			}
		}
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Option does not belong to this poll."})
			return
		}

		key := "poll:" + pollID.Hex() + ":option:" + strconv.Itoa(request.OptionID)
		if err := redisClient.Incr(c.Request.Context(), key).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not record vote."})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Vote recorded successfully!"})
	})

	router.GET("/api/polls/:id/results", func(c *gin.Context) {
		pollID, err := bson.ObjectIDFromHex(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid poll ID."})
			return
		}
		var poll Poll
		if err := polls.FindOne(c.Request.Context(), bson.M{"_id": pollID}).Decode(&poll); err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Poll not found."})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve poll."})
			return
		}

		results := make([]gin.H, 0, len(poll.Options))
		var total int64
		for _, option := range poll.Options {
			key := "poll:" + pollID.Hex() + ":option:" + strconv.Itoa(option.ID)
			count, err := redisClient.Get(c.Request.Context(), key).Int64()
			if err != nil && err != redis.Nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve vote counts."})
				return
			}
			total += count
			results = append(results, gin.H{"optionId": option.ID, "text": option.Text, "votes": count})
		}
		c.JSON(http.StatusOK, gin.H{
			"pollId": pollID.Hex(), "question": poll.Question,
			"totalVotes": total, "results": results,
		})
	})

	port := os.Getenv("PORT")
if port == "" {
    port = "8080"
}

if err := router.Run(":" + port); err != nil {
    log.Fatal("Server failed: ", err)
}
}
