/* ui.js —— 展示层：报价卡 / 情绪面板 / 资讯 / 热门 / 基本面评分 渲染 */
const sentiLabel = { bull: '看多', bear: '看空', neutral: '中性' };
const scoreClass = n => n <= 3 ? 'low' : (n <= 7 ? 'mid' : 'high');

/* ---- 情绪面板（zhihu_search 数据渲染） ---- */
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
  const pct = Math.round((val || 0) * 100);
  return '<div class="ratio-row"><span class="r-label">' + label + '</span>' +
    '<span class="r-bar"><span class="r-fill ' + cls + '" style="width:' + pct + '%"></span></span>' +
    '<span class="r-pct">' + pct + '%</span></div>';
}

/* ---- 实时报价卡（fetchKline 调用） ---- */
function renderQuote(q) {
  const lvl = chgLevel(q.change_pct);
  const up = (q.change || 0) >= 0;
  const sign = up ? '+' : '';
  const vol = (q.volume || 0) >= 10000 ? (q.volume / 10000).toFixed(2) + '万手' : (q.volume || 0) + '手';
  $('quote-body').innerHTML =
    '<div class="q-row">' +
      '<div>' +
        '<span class="q-price ' + lvl + '">' + (q.price || 0).toFixed(2) + '</span>' +
        '<span class="q-change ' + lvl + '">' + sign + (q.change || 0).toFixed(2) + '  ' + sign + (q.change_pct || 0).toFixed(2) + '%</span>' +
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

function renderNews(items) {
  if (!items || !items.length) return; // 无资讯不显示卡片
  let html = '<div class="news-list">';
  for (const it of items) {
    const u = it.url || '';
    html += '<div class="news-item">' +
      (u ? '<a class="t" href="' + esc(u) + '" target="_blank" rel="noopener" title="打开原文">' + esc(it.title || '(无标题)') + '</a>'
          : '<span class="t">' + esc(it.title || '(无标题)') + '</span>') +
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

/* ---- 板块 chip（热门板块 / 暴跌板块复用，点击查看成分股） ---- */
function renderHotSectors(containerId, cardId, items) {
  const el = $(containerId);
  if (!items || !items.length) { $(cardId).classList.add('hidden'); return; }
  let html = '';
  for (const it of items) {
    const lvl = chgLevel(it.change_pct);
    const up = lvl !== 'down';
    const n = String(it.name || '').replace(/'/g, '');
    html += '<span class="hot-chip" onclick="hotSectorClick(\'' + esc(it.code) + '\',\'' + esc(n) + '\')" title="查看板块成分股">' +
      '<span class="hn">' + esc(it.name) + '</span>' +
      '<span class="hp ' + lvl + '">' + (up ? '+' : '') + (it.change_pct || 0).toFixed(2) + '%</span></span>';
  }
  el.innerHTML = html;
  $(cardId).classList.remove('hidden');
}

/* ---- 热门股票（行，点击查询） ---- */
function renderHotStocks(items) {
  const el = $('hot-stocks');
  if (!items || !items.length) { $('hot-stocks-card').classList.add('hidden'); return; }
  let html = '';
  for (const it of items) {
    const lvl = chgLevel(it.change_pct);
    const up = lvl !== 'down';
    const hn = String(it.name || '').replace(/'/g, '');
    html += '<div class="hot-stock" onclick="hotSearch(\'' + esc(hn) + '\')" title="点击查询 ' + esc(it.name) + '">' +
      '<span class="sn">' + esc(it.name) + '</span>' +
      '<span class="sc">' + esc(it.code) + '</span>' +
      '<span class="sp">' + (it.price || 0).toFixed(2) + '</span>' +
      '<span class="spct ' + lvl + '">' + (up ? '+' : '') + (it.change_pct || 0).toFixed(2) + '%</span>' +
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
    const lvl = chgLevel(it.change_pct);
    const up = lvl !== 'down';
    const n = String(it.name || '').replace(/'/g, '');
    group += '<span class="ticker-item" onclick="hotSearch(\'' + esc(n) + '\')">' +
      '<span class="tn">' + esc(it.name) + '</span>' +
      '<span class="row2">' +
        '<span class="tp">' + (it.price || 0).toFixed(2) + '</span>' +
        '<span class="td ' + lvl + '">' + (up ? '+' : '') + (it.change_pct || 0).toFixed(2) + '%</span>' +
      '</span>' +
      '</span>';
  }
  // 双份内容实现无缝滚动
  el.innerHTML = '<div class="ticker-track">' + group + group + '</div>';
}

/* ---- 基本面评分渲染（ask 的 fundamental 事件） ---- */
function renderFundamental(d) {
  const el = $('fundamental-score');
  if (!el) return;
  if (!d || !d.score) { el.innerHTML = '<span class="fin-loading">暂无评分数据</span>'; return; }
  const sc = d.score, v = d.valuation || {};
  const dims = [
    ['盈利能力', sc.profit], ['成长性', sc.growth],
    ['财务健康', sc.health], ['估值', sc.valuation],
  ];
  let rows = '';
  for (const [name, val] of dims) {
    const cls = val >= 75 ? 'up3' : (val >= 60 ? 'up2' : (val >= 40 ? 'up1' : 'down'));
    rows += '<div class="fd-row"><span class="fd-name">' + name + '</span>' +
      '<span class="fd-bar"><span class="fd-fill ' + cls + '" style="width:' + val + '%"></span></span>' +
      '<span class="fd-val ' + cls + '">' + val + '</span></div>';
  }
  const total = sc.total || 0;
  const grade = sc.grade || '';
  const tcls = total >= 75 ? 'up3' : (total >= 60 ? 'up2' : (total >= 40 ? 'up1' : 'down'));
  const valLines = [
    'PE(TTM) ' + (v.pe ? v.pe.toFixed(2) : '—') +
      (v.pe_ent_percent >= 0 ? '（历史分位 ' + v.pe_ent_percent.toFixed(0) + '%）' : '（分位不可用）'),
    'PB ' + (v.pb ? v.pb.toFixed(2) : '—') +
      (v.market_cap ? ' · 市值 ' + v.market_cap.toFixed(0) + '亿' : ''),
  ];
  el.innerHTML =
    '<div class="fd-total"><span class="fd-t-num ' + tcls + '">' + total + '</span>' +
    '<span class="fd-t-grade ' + tcls + '">' + esc(grade) + '</span></div>' +
    '<div class="fd-dims">' + rows + '</div>' +
    '<div class="fd-val-line">' + valLines.map(esc).join(' · ') + '</div>' +
    (sc.no_data && sc.no_data.length ? '<div class="fd-nodata">数据不足维度：' + esc(sc.no_data.join('、')) + '（评分按可用维度归一）</div>' : '');
  $('fundamental-card').classList.remove('hidden');
}
