/* watchlist.js —— 自选池（20 只上限，本地持久化；列表聚合基本面评分） */
function loadWatchlist() {
  fetch('/api/watchlist').then(r => r.json()).then(items => {
    if (!Array.isArray(items)) throw new Error('bad');
    const remain = 20 - items.length;
    const rm = $('watchlist-remain');
    if (rm) rm.textContent = remain > 0 ? '还可添加 ' + remain + ' 只' : '已满 20/20';
    const list = $('watchlist-list');
    if (!items.length) {
      list.innerHTML = '<span class="fin-loading">暂无自选，输入代码添加</span>';
      return;
    }
    let html = '';
    for (const it of items) {
      html += '<div class="wl-item" data-code="' + esc(it.code) + '">' +
        '<span class="wl-name">' + esc(it.code) + '</span>' +
        '<span class="wl-score fin-loading">评分中…</span>' +
        '<span class="wl-rm" onclick="removeWatchlist(\'' + esc(it.code) + '\')" title="移除">×</span></div>';
    }
    list.innerHTML = html;
    items.forEach(it => loadWatchScore(it.code, it.market)); // MVP：逐只聚合评分
  }).catch(() => {
    const list = $('watchlist-list');
    if (list) list.innerHTML = '<span class="fin-loading">自选池加载失败</span>';
  });
}

async function loadWatchScore(code, market) {
  try {
    const resp = await fetch('/api/fundamental?code=' + encodeURIComponent(code) + '&market=' + encodeURIComponent(market || ''));
    const d = await resp.json();
    const row = document.querySelector('.wl-item[data-code="' + code + '"]');
    if (!row || !d || !d.score) return;
    const total = d.score.total, grade = d.score.grade || '';
    const cls = total >= 75 ? 'up3' : (total >= 60 ? 'up2' : (total >= 40 ? 'up1' : 'down'));
    row.querySelector('.wl-name').textContent = d.name || code;
    row.querySelector('.wl-score').innerHTML = total + ' <em>' + esc(grade) + '</em>';
    row.querySelector('.wl-score').className = 'wl-score ' + cls;
    row.classList.add('clickable');
    row.setAttribute('onclick', "searchWatch('" + esc(code) + "')");
  } catch (e) { /* 单只失败静默 */ }
}

function addWatchlist() {
  const input = $('watchlist-input');
  const code = (input.value || '').trim();
  if (!code) return;
  fetch('/api/watchlist/add', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code, market: guessMarket(code) }),
  }).then(r => r.json()).then(d => {
    if (d.error) { alert(d.error); return; }
    input.value = '';
    loadWatchlist();
  }).catch(() => alert('添加失败'));
}

function removeWatchlist(code) {
  fetch('/api/watchlist/remove', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  }).then(() => loadWatchlist());
}

/* 详情页「加入自选」（stock 事件记录 curStock） */
function addToWatchlist() {
  if (!curStock) return;
  fetch('/api/watchlist/add', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code: curStock.code, market: curStock.market }),
  }).then(r => r.json()).then(d => {
    if (d.error) alert(d.error); else toast('已加入自选');
  }).catch(() => alert('添加失败'));
}

function searchWatch(code) {
  $('stock-input').value = code;
  hideSuggest();
  doSearch();
}

function showWatchlistHelp() {
  alert('自选池最多 20 只，保存在本机（data/watchlist.json）。\n添加：上方输入代码，或详情页「+自选」；\n点击列表项可查看该股基本面。');
}

function guessMarket(code) {
  code = String(code || '');
  if (/^(6|9)/.test(code)) return '沪A';
  if (/^(0|3)/.test(code)) return '深A';
  if (/^(4|8)/.test(code)) return '北交';
  return '';
}
