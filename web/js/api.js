/* api.js —— API 层：后端接口封装（股票识别 / 行情 / SSE 分析） */
async function apiResolve(q) {
  const resp = await fetch('/api/resolve?q=' + encodeURIComponent(q));
  if (!resp.ok) return null;
  return resp.json();
}

async function apiKline(code, market, days) {
  const resp = await fetch('/api/kline?code=' + encodeURIComponent(code) +
    '&market=' + encodeURIComponent(market || '沪A') + '&days=' + (days || 60));
  if (!resp.ok) return null;
  return resp.json();
}

async function apiNews(keyword, count) {
  const resp = await fetch('/api/news?keyword=' + encodeURIComponent(keyword) +
    '&count=' + (count || 5));
  if (!resp.ok) return null;
  return resp.json();
}

async function apiAsk(stock) {
  return fetch('/api/ask', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ stock })
  });
}

async function apiHot(type, count, code) {
  let u = '/api/hot?type=' + encodeURIComponent(type) + '&count=' + (count || 8);
  if (code) u += '&code=' + encodeURIComponent(code);
  const resp = await fetch(u);
  if (!resp.ok) return null;
  return resp.json();
}

async function apiKnowledge(q, limit) {
  const resp = await fetch('/api/knowledge?q=' + encodeURIComponent(q) + '&limit=' + (limit || 10));
  if (!resp.ok) return null;
  return resp.json();
}

/* 二期：与看山对话 */
async function apiChat(code, market, message) {
  return fetch('/api/chat', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ stock: code, market: market || '', message })
  });
}

async function apiChatReset(code) {
  const resp = await fetch('/api/chat/reset', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ stock: code })
  });
  return resp.ok;
}
