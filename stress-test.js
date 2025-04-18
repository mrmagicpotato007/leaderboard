import http from 'k6/http';
import { check, sleep } from 'k6';

export let options = {
  vus: 100,   // virtual users
  duration: '30s',  // test duration
};

const genUser = (index) => {
  return {
    username: `user_${index}_${Math.random().toString(36).substring(7)}`,
    password: `pass_${Math.random().toString(36).substring(10)}`
  };
};

export default function () {
  const user = genUser(__VU);

  // 1. Signup
  let signupRes = http.post('http://localhost:8084/v1/signup', JSON.stringify(user), {
    headers: { 'Content-Type': 'application/json' },
  });

  check(signupRes, { 'signed up': (r) => r.status === 200 });

  // 2. Login
  let loginRes = http.post('http://localhost:8084/v1/login', JSON.stringify(user), {
    headers: { 'Content-Type': 'application/json' },
  });

  check(loginRes, { 'logged in': (r) => r.status === 200 });

  const token = loginRes.json('token') || loginRes.json('access_token');
  if (!token) return;

  const authHeader = { 'Authorization': `Bearer ${token}`, 'Content-Type': 'application/json' };

  // 3. Submit score
  http.post('http://localhost:8085/v1/score', JSON.stringify({
    score: Math.floor(Math.random() * 1000),
    game_mode: 'classic'
  }), { headers: authHeader });

  // 4. Get leaderboard
  http.get('http://localhost:8086/v1/leaderboard/top', { headers: authHeader });

  // 5. Get rank
  http.get(`http://localhost:8086/v1/rank/1`, { headers: authHeader });

  sleep(1);
}
