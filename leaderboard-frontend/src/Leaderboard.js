import React from "react";

function Leaderboard({ data, loading, error }) {
  if (loading) return <div>Loading leaderboard...</div>;
  if (error) return <div style={{ color: "red" }}>{error}</div>;
  if (!data || data.length === 0) return <div>No scores found.</div>;
  return (
    <table style={{ width: "100%", borderCollapse: "collapse" }}>
      <thead>
        <tr>
          <th style={{ borderBottom: "1px solid #ccc" }}>Rank</th>
          <th style={{ borderBottom: "1px solid #ccc" }}>User ID</th>
          <th style={{ borderBottom: "1px solid #ccc" }}>Score</th>
        </tr>
      </thead>
      <tbody>
        {data.map((row, i) => (
          <tr key={row.user_id || i}>
            <td style={{ textAlign: "center" }}>{i + 1}</td>
            <td style={{ textAlign: "center" }}>{row.user_id}</td>
            <td style={{ textAlign: "center" }}>{row.score}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export default Leaderboard;
