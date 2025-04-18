import React, { useEffect, useState } from "react";
import Leaderboard from "./Leaderboard";
import SearchRank from "./SearchRank";

// No need for API_BASE with proxy - use relative URLs
const BEARER = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NDUwMzgxMDIsInVzZXJfaWQiOjEsInVzZXJuYW1lIjoidGVzdHVzZXIifQ.G9hCFICVDBKjkoI4IRxtJ4UsjcKludVFbvD7oTDjtPk";

function App() {
  const [leaderboard, setLeaderboard] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let interval = null;
    const fetchLeaderboard = async () => {
      setLoading(true);
      setError("");
      try {
        const res = await fetch(`/v1/leaderboard/top`, {
          method: 'GET',
          headers: {
            Authorization: `Bearer ${BEARER}`,
            "Content-Type": "application/json",
            "Accept": "application/json",
          },
          mode: 'cors', // Explicitly request CORS mode
        });
        
        if (!res.ok) {
          const errorText = await res.text();
          throw new Error(`Failed to fetch leaderboard: ${res.status} ${errorText}`);
        }
        
        const data = await res.json();
        setLeaderboard(data);
      } catch (err) {
        console.error("Leaderboard fetch error:", err);
        setError(err.message || "Network error when fetching leaderboard");
      } finally {
        setLoading(false);
      }
    };
    fetchLeaderboard();
    interval = setInterval(fetchLeaderboard, 5000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div style={{ maxWidth: 600, margin: "40px auto", fontFamily: "sans-serif" }}>
      <h2>Top Scores Leaderboard</h2>
      <Leaderboard data={leaderboard} loading={loading} error={error} />
      <hr />
      <SearchRank />
    </div>
  );
}

export default App;
