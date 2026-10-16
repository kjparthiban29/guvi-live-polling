
import { useEffect, useState } from "react";
import "./App.css";

const API = "http://localhost:8080";

function App() {
  const [mode, setMode] = useState("signup");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [token, setToken] = useState("");
  const [question, setQuestion] = useState("");
  const [options, setOptions] = useState(["", ""]);
  const [pollId, setPollId] = useState("");
  const [poll, setPoll] = useState(null);
  const [results, setResults] = useState(null);
  const [selectedOption, setSelectedOption] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  async function request(path, method = "GET", body, authenticated = false) {
    const headers = {};
    if (body) headers["Content-Type"] = "application/json";
    if (authenticated) headers.Authorization = `Bearer ${token}`;

    const response = await fetch(`${API}${path}`, {
      method,
      headers,
      ...(body ? { body: JSON.stringify(body) } : {}),
    });

    const data = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(data.error || data.message || "Request failed");
    }
    return data;
  }

  async function authenticate(event) {
    event.preventDefault();
    setBusy(true);
    setMessage("");

    try {
      const data = await request(
        `/api/auth/${mode}`,
        "POST",
        { email, password }
      );
      setToken(data.token);
      setMessage(`Successfully ${mode === "signup" ? "signed up" : "logged in"}! You can now create a poll.`);
    } catch (error) {
      setMessage(error.message);
    } finally {
      setBusy(false);
    }
  }

  async function createPoll(event) {
    event.preventDefault();
    setBusy(true);
    setMessage("");

    try {
      const data = await request(
        "/api/polls",
        "POST",
        {
          question,
          options: options.map((item) => item.trim()).filter(Boolean),
        },
        true
      );

      const createdPoll = data.poll || data;
      const id = createdPoll.id || createdPoll._id;
      if (!id) throw new Error("Poll created, but no poll ID was returned.");

      setPollId(id);
      setMessage("Poll created successfully!");
      await loadPoll(id);
    } catch (error) {
      setMessage(error.message);
    } finally {
      setBusy(false);
    }
  }

  async function loadPoll(id = pollId) {
    if (!id.trim()) {
      setMessage("Please enter a poll ID.");
      return;
    }

    setBusy(true);
    setMessage("");

    try {
      const data = await request(`/api/polls/${id.trim()}`);
      setPoll(data.poll || data);
      setPollId(id.trim());
      await loadResults(id.trim());
    } catch (error) {
      setMessage(error.message);
    } finally {
      setBusy(false);
    }
  }

  async function loadResults(id = pollId) {
    try {
      const data = await request(`/api/polls/${id}/results`);
      setResults(data);
    } catch (error) {
      setMessage(error.message);
    }
  }
  useEffect(() => {
    if (!pollId || !poll) return;

    const timer = setInterval(() => {
      loadResults(pollId);
    }, 2000);

    return () => clearInterval(timer);
  }, [pollId, poll]);

  async function vote(event) {
    event.preventDefault();
    if (selectedOption === "") {
      setMessage("Please select an option.");
      return;
    }

    setBusy(true);
    setMessage("");

    try {
      await request(`/api/polls/${pollId}/vote`, "POST", {
        optionId: Number(selectedOption),
      });
      await loadResults(pollId);
      setMessage("Your vote was submitted!");
    } catch (error) {
      setMessage(error.message);
    } finally {
      setBusy(false);
    }
  }

  async function copyPollId() {
    try {
      await navigator.clipboard.writeText(pollId);
      setMessage("Poll ID copied!");
    } catch {
      setMessage(`Your poll ID is: ${pollId}`);
    }
  }

  const resultItems = results?.results || results?.options || [];

  return (
    <main className="app">
      <header className="header">
        <div className="logo">QuickPoll</div>
        <p>Ask a question. Collect votes. See results.</p>
      </header>

      <section className="card">
        <h2>{token ? "You're signed in" : "Welcome to QuickPoll"}</h2>

        {!token ? (
          <>
            <div className="auth-tabs">
              <button
                type="button"
                className={mode === "signup" ? "active" : ""}
                onClick={() => setMode("signup")}
              >
                Sign up
              </button>
              <button
                type="button"
                className={mode === "login" ? "active" : ""}
                onClick={() => setMode("login")}
              >
                Log in
              </button>
            </div>

            <form onSubmit={authenticate}>
              <label>Email</label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
                required
              />

              <label>Password (at least 8 characters)</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                minLength={8}
                required
              />

              <button className="primary" disabled={busy}>
                {busy ? "Please wait..." : mode === "signup" ? "Create account" : "Log in"}
              </button>
            </form>
          </>
        ) : (
          <>
            <p>Signed in as {email}</p>
            <button
              type="button"
              onClick={() => {
                setToken("");
                setPassword("");
                setMessage("You have logged out.");
              }}
            >
              Log out
            </button>
          </>
        )}
      </section>

      {token && (
        <section className="card">
          <h2>Create a poll</h2>
          <form onSubmit={createPoll}>
            <label>Your question</label>
            <input
              value={question}
              onChange={(e) => setQuestion(e.target.value)}
              placeholder="What should we learn next?"
              required
              maxLength={200}
            />

            <label>Answer options</label>
            {options.map((option, index) => (
              <input
                key={index}
                value={option}
                onChange={(e) =>
                  setOptions((current) =>
                    current.map((item, i) => i === index ? e.target.value : item)
                  )
                }
                placeholder={`Option ${index + 1}`}
                required
                maxLength={100}
              />
            ))}

            {options.length < 10 && (
              <button
                type="button"
                onClick={() => setOptions((current) => [...current, ""])}
              >
                + Add option
              </button>
            )}

            {options.length > 2 && (
              <button
                type="button"
                onClick={() => setOptions((current) => current.slice(0, -1))}
              >
                Remove last option
              </button>
            )}

            <button className="primary" disabled={busy}>
              Create poll
            </button>
          </form>
        </section>
      )}

      <section className="card">
        <h2>Join a poll</h2>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            loadPoll(pollId);
          }}
        >
          <label>Poll ID</label>
          <input
            value={pollId}
            onChange={(e) => setPollId(e.target.value)}
            placeholder="Paste a poll ID"
            required
          />
          <button className="primary" disabled={busy}>
            Open poll
          </button>
        </form>

        {poll && (
          <div className="poll-area">
            <h3>{poll.question}</h3>
            <form onSubmit={vote}>
              {(poll.options || []).map((option, index) => (
                <label className="option" key={index}>
                  <input
                    type="radio"
                    name="poll-option"
                    value={index + 1}
                    checked={selectedOption === String(index + 1)}
                    onChange={(e) => setSelectedOption(e.target.value)}
                  />
                  {typeof option === "string" ? option : option.text || option.name}
                </label>
              ))}
              <button className="primary" disabled={busy}>
                Submit vote
              </button>
            </form>

            <button type="button" onClick={() => loadResults(pollId)}>
              Refresh results
            </button>

            <h3>Live results</h3>
            {resultItems.map((item, index) => {
              const label = item.text ?? item.option ?? item.name ?? `Option ${index + 1}`;
              const votes = Number(item.votes ?? item.count ?? 0);
              const total = Number(results?.totalVotes ?? 0);
              const percent = total ? Math.round((votes / total) * 100) : 0;

              return (
                <div className="result" key={index}>
                  <div className="result-label">
                    <span>{label}</span>
                    <span>{votes} votes ({percent}%)</span>
                  </div>
                  <div className="bar">
                    <div className="bar-fill" style={{ width: `${percent}%` }} />
                  </div>
                </div>
              );
            })}

            <button type="button" onClick={copyPollId}>
              Copy poll ID
            </button>
          </div>
        )}
      </section>

      {message && <p className="message">{message}</p>}
    </main>
  );
}

export default App;
