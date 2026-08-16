/* ui.js —— 展示层：报价卡 / 情绪面板 / 错误提示 渲染 */
const sentiLabel = { bull: '看多', bear: '看空', neutral: '中性' };
const scoreClass = n => n <= 3 ? 'low' : (n <= 7 ? 'mid' : 'high');

/* ---- 报价卡 ---- */
function renderQuote(q) {
  const up = (q.change || 0) >= 0;
  const color = up ? 'var(--bull)' : 'var(--bear)';
  const sign = up ? '+' : '';
  const vol = (q.volume || 0) >= 10000 ? (q.volume / 10000).toFixed(2) + '万手' : (q.volume || 0) + '手';
  $('quote-body').innerHTML =
    '<div class="q-row">' +
      '<div>' +
        '<span class="q-price" style="color:' + color + '">' + (q.price || 0).toFixed(2) + '</span>' +
        '<span class="q-change" style="color:' + color + '">' + sign + (q.change || 0).toFixed(2) + '  ' + sign + (q.change_pct || 0).toFixed(2) + '%</span>' +
      '</div>' +
      '<div class="q-grid">' +
        qCell('今开', (q.open || 0).toFixed(2)) +
        qCell('最高', (q.high || 0).toFixed(2)) +
        qCell('最低', (q.low || 0).toFixed(2)) +
        qCell('昨收', (q.prev_close || 0).toFixed(2)) +
        qCell('成交量', vol) +
      '</div>' +
    '</div>';
  $('quote-card').classList.remove('hidden');
}
function qCell(k, v) {
  return '<div class="q-cell"><div class="k">' + k + '</div><div class="v">' + v + '</div></div>';
}

/* ---- 情绪面板 ---- */
function renderSentiment(s) {
  if (!s) return;
  let html = '';
  if (s.degraded) {
    html += '<div class="degraded">' + esc(s.err_msg || '数据不足，已降级展示') + '</div>';
  }
  html += '<div class="heat"><span class="num">' + (s.heat || 0) + '</span>' +
    '<span class="label">条近期讨论</span>' +
    '<span class="sample">样本 ' + (s.sample || 0) + ' 条</span></div>';

  const r = s.ratio || {};
  html += ratioRow('看多', 'bull', r.bull) + ratioRow('看空', 'bear', r.bear) + ratioRow('中性', 'neutral', r.neutral);

  if (s.score != null) {
    html += '<div class="score-box">' +
      '<span class="score-num ' + scoreClass(s.score) + '">' + s.score + '<small style="font-size:14px;color:var(--faint)">/10</small></span>' +
      '<span class="score-note">参考强度：反映当前讨论情绪与数据的充分程度，不代表涨跌预测。</span></div>';
  }

  const items = s.items || [];
  if (items.length) {
    html += '<div class="section-title" style="margin-top:18px">代表观点</div><div class="items">';
    for (const it of items) {
      html += '<div class="item">' +
        '<div class="title-line">' +
        '<a href="' + esc(it.url || '#') + '" target="_blank" rel="noopener">' + esc(it.title || '(无标题)') + '</a>' +
        '<span class="tag-sentiment ' + esc(it.sentiment || 'neutral') + '">' + esc(sentiLabel[it.sentiment] || '中性') + '</span>' +
        '</div>' +
        '<div class="meta"><span>✍️ ' + esc(it.author || '匿名') + '</span><span>👍 ' + (it.vote_up || 0) + '</span></div>' +
        '<div class="excerpt">' + esc(it.excerpt || '') + '</div>' +
        '</div>';
    }
    html += '</div>';
  }
  $('sentiment-body').innerHTML = html;
}

function ratioRow(label, cls, val) {
  const p = Math.round((val || 0) * 100);
  return '<div class="ratio-row"><span class="rlabel">' + label + '</span>' +
    '<div class="bar"><div class="' + cls + '" style="width:' + p + '%"></div></div>' +
    '<span class="rpct">' + p + '%</span></div>';
}

/* ---- 相关资讯（辅助，仅参考展示，不跳转外部） ---- */
function renderNews(items) {
  if (!items || !items.length) return; // 无资讯不显示卡片
  let html = '<div class="news-list">';
  for (const it of items) {
    html += '<div class="news-item">' +
      '<span class="t" title="外部资讯仅供参考，不提供跳转">' + esc(it.title || '(无标题)') + '</span>' +
      '<span class="date">' + esc((it.date || '').slice(0, 10)) + '</span>' +
      '<span class="src">' + esc(it.source || '') + '</span>' +
      '</div>';
  }
  html += '</div>';
  $('news-body').innerHTML = html;
  $('news-card').classList.remove('hidden');
}

/* ---- 错误 ---- */
function showError(msg) {
  const box = $('error-box');
  box.classList.remove('hidden');
  box.querySelector('.err-box').innerHTML =
    '<strong>😥 出错了，请稍后重试。</strong>' +
    '<div class="detail">' + esc(msg || '未知错误') + '</div>';
}
function hideError() { $('error-box').classList.add('hidden'); }

/* ---- 热门板块（chip） ---- */
function renderHotSectors(items) {
  const el = $('hot-sectors');
  if (!items || !items.length) { $('hot-sectors-card').classList.add('hidden'); return; }
  let html = '';
  for (const it of items) {
    const up = (it.change_pct || 0) >= 0;
    const n = String(it.name || '').replace(/'/g, '');
    html += '<span class="hot-chip" onclick="hotSectorClick(\'' + esc(it.code) + '\',\'' + esc(n) + '\')" title="查看板块成分股">' +
      '<span class="hn">' + esc(it.name) + '</span>' +
      '<span class="hp ' + (up ? 'up' : 'down') + '">' + (up ? '+' : '') + (it.change_pct || 0).toFixed(2) + '%</span></span>';
  }
  el.innerHTML = html;
  $('hot-sectors-card').classList.remove('hidden');
}

/* ---- 热门股票（行，点击查询） ---- */
function renderHotStocks(items) {
  const el = $('hot-stocks');
  if (!items || !items.length) { $('hot-stocks-card').classList.add('hidden'); return; }
  let html = '';
  for (const it of items) {
    const up = (it.change_pct || 0) >= 0;
    html += '<div class="hot-stock" onclick="hotSearch(\'' + esc(it.name) + '\')" title="点击查询 ' + esc(it.name) + '">' +
      '<span class="sn">' + esc(it.name) + '</span>' +
      '<span class="sc">' + esc(it.code) + '</span>' +
      '<span class="sp">' + (it.price || 0).toFixed(2) + '</span>' +
      '<span class="spct ' + (up ? 'up' : 'down') + '">' + (up ? '+' : '') + (it.change_pct || 0).toFixed(2) + '%</span>' +
      '</div>';
  }
  el.innerHTML = html;
  $('hot-stocks-card').classList.remove('hidden');
}

/* ---- ticker 行情条 ---- */
function renderTicker(items) {
  const el = $('ticker-bar');
  if (!items || !items.length) { el.innerHTML = ''; return; }
  let group = '';
  for (const it of items) {
    const up = (it.change_pct || 0) >= 0;
    const n = String(it.name || '').replace(/'/g, '');
    group += '<span class="ticker-item" onclick="hotSearch(\'' + esc(n) + '\')">' +
      '<span class="tn">' + esc(it.name) + '</span>' +
      '<span class="tp ' + (up ? 'up' : 'down') + '">' + (up ? '+' : '') + (it.change_pct || 0).toFixed(2) + '%</span>' +
      '</span>';
  }
  // 双份内容实现无缝滚动
  el.innerHTML = '<div class="ticker-track">' + group + group + '</div>';
}

/* ---- 知乎热榜（主数据源） ---- */
function renderZhihuHot(items) {
  const el = $('zhihu-hot');
  if (!items || !items.length) { $('zhihu-hot-card').classList.add('hidden'); return; }
  let html = '<div class="news-list">';
  items.forEach((it, i) => {
    html += '<div class="zhihu-hot-item">' +
      '<span class="rank">' + (i + 1) + '</span>' +
      '<a href="' + esc(it.url || '#') + '" target="_blank" rel="noopener">' + esc(it.title || '') + '</a>' +
      '</div>';
  });
  html += '</div>';
  el.innerHTML = html;
  $('zhihu-hot-card').classList.remove('hidden');
}

/* ---- 热门辅助数据：友好降级 ---- */
function showHotDegraded(type) {
  const id = type === 'stock' ? 'hot-stocks' : 'hot-sectors';
  const card = type === 'stock' ? 'hot-stocks-card' : 'hot-sectors-card';
  $('hot-stocks-label') && ($('hot-stocks-label').textContent = '');
  $('hot-stocks-back') && $('hot-stocks-back').classList.add('hidden');
  $(id).innerHTML = '<div class="degraded" style="margin-top:8px">热门行情数据暂时不可用，请稍后重试。' +
    '<a style="margin-left:8px;cursor:pointer" onclick="retryHot()">重试</a></div>';
  $(card).classList.remove('hidden');
}
