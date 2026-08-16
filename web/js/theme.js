/* theme.js —— 明暗主题（localStorage 记忆 + 跟随系统偏好） */
function applyTheme(t) {
  document.documentElement.setAttribute('data-theme', t);
  localStorage.setItem('zhihudp-theme', t);
  $('theme-btn').textContent = t === 'dark' ? '☀️' : '🌙';
}

function toggleTheme() {
  const cur = document.documentElement.getAttribute('data-theme');
  applyTheme(cur === 'dark' ? 'light' : 'dark');
}

(function initTheme() {
  const saved = localStorage.getItem('zhihudp-theme');
  const preferDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
  applyTheme(saved || (preferDark ? 'dark' : 'light'));
})();
