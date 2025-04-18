import React, { useState } from "react";

// No need for API_BASE with proxy - use relative URLs
const BEARER = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NDUwMzgxMDIsInVzZXJfaWQiOjEsInVzZXJuYW1lIjoidGVzdHVzZXIifQ.G9hCFICVDBKjkoI4IRxtJ4UsjcKludVFbvD7oTDjtPk";

function SearchRank() {
  const [userId, setUserId] = useState("");
  const [result, setResult] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);
    setError("");
    setResult(null);
    try {
      const res = await fetch(`/v1/rank/${userId}`, {
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
        throw new Error(`Failed to fetch rank: ${res.status} ${errorText}`);
      }
      
      const data = await res.json();
      setResult(data);
    } catch (err) {
      console.error("Rank fetch error:", err);
      setError(err.message || "Network error when fetching rank");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ marginTop: 32 }}>
      <h3>Search User Rank</h3>
      <form onSubmit={handleSubmit} style={{ display: "flex", gap: 8 }}>
        <input
          type="text"
          placeholder="Enter User ID"
          value={userId}
          onChange={(e) => setUserId(e.target.value)}
          style={{ flex: 1 }}
        />
        <button type="submit" disabled={loading || !userId}>
          {loading ? "Searching..." : "Get Rank"}
        </button>
      </form>
      {error && <div style={{ color: "red", marginTop: 8 }}>{error}</div>}
      {result && (
        <div style={{ marginTop: 12 }}>
          <b>User ID:</b> {result.user_id} <br />
          <b>Rank:</b> {result.rank} <br />
          <b>Score:</b> {result.score}
        </div>
      )}
    </div>
  );
}

export default SearchRank;
