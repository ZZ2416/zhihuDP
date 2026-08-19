/* kline.js —— K线图渲染与交互（SVG 蜡烛图 + MA均线 + 成交量 + 十字光标/明细浮层）
 * 交互：悬停/触摸显示当日 开收高低/成交量/涨跌幅（详见 features/kline/INTERACTION.md）
 */
function renderKline(candles, quote) {
  const el = $('kline-chart');
  if (!candles || !candles.length) {
    el.innerHTML = '<div class="degraded" style="margin-top:12px">暂无行情数据</div>';
    return;
  }
  const n = candles.length;
  const W = 860, H = 340, padL = 58, padR = 12, padT = 18;
  const volH = 60, dateH = 26;
  const priceH = H - volH - dateH - padT;
  const plotW = W - padL - padR;
  const cw = plotW / n;
  const bodyW = Math.max(1, cw * 0.6);

  let min = Infinity, max = -Infinity, maxVol = 0;
  for (const c of candles) {
    if (c.low < min) min = c.low;
    if (c.high > max) max = c.high;
    if (c.volume > maxVol) maxVol = c.volume;
  }
  const pad = (max - min) * 0.05 || 1;
  min -= pad; max += pad;
  const y = v => padT + (max - v) / (max - min) * priceH;
  const x = i => padL + i * cw + cw / 2;
  const ma = k => {
    const pts = [];
    for (let i = k - 1; i < n; i++) {
      let s = 0;
      for (let j = i - k + 1; j <= i; j++) s += candles[j].close;
      pts.push(x(i).toFixed(1) + ',' + y(s / k).toFixed(1));
    }
    return pts.join(' ');
  };

  let svg = '';
  // 网格 + 价格刻度
  const gl = 4;
  for (let i = 0; i <= gl; i++) {
    const v = min + (max - min) * i / gl, yy = y(v);
    svg += '<line x1="' + padL + '" y1="' + yy + '" x2="' + (W - padR) + '" y2="' + yy + '" stroke="var(--border)" stroke-width="1"/>';
    svg += '<text x="' + (padL - 6) + '" y="' + (yy + 4) + '" text-anchor="end" font-size="10" fill="var(--faint)">' + v.toFixed(2) + '</text>';
  }
  // 蜡烛（红涨绿跌）
  for (let i = 0; i < n; i++) {
    const c = candles[i];
    const up = c.close >= c.open;
    const color = up ? 'var(--bull)' : 'var(--bear)';
    const cx = x(i);
    svg += '<line x1="' + cx + '" y1="' + y(c.high) + '" x2="' + cx + '" y2="' + y(c.low) + '" stroke="' + color + '" stroke-width="1"/>';
    const yo = y(c.open), yc = y(c.close);
    const top = Math.min(yo, yc), h = Math.max(1, Math.abs(yo - yc));
    svg += '<rect x="' + (cx - bodyW / 2) + '" y="' + top + '" width="' + bodyW + '" height="' + h + '" fill="' + color + '"/>';
  }
  // 均线 MA5/10/20
  svg += '<polyline points="' + ma(5) + '" fill="none" stroke="#f7b731" stroke-width="1.4"/>';
  svg += '<polyline points="' + ma(10) + '" fill="none" stroke="#3867d6" stroke-width="1.4"/>';
  svg += '<polyline points="' + ma(20) + '" fill="none" stroke="#8854d0" stroke-width="1.4"/>';
  // 成交量
  const volTop = H - dateH;
  svg += '<line x1="' + padL + '" y1="' + volTop + '" x2="' + (W - padR) + '" y2="' + volTop + '" stroke="var(--border)"/>';
  for (let i = 0; i < n; i++) {
    const c = candles[i];
    const up = c.close >= c.open;
    const color = up ? 'var(--bull)' : 'var(--bear)';
    const h = Math.max(1, c.volume / maxVol * volH);
    svg += '<rect x="' + (x(i) - bodyW / 2) + '" y="' + (volTop - h) + '" width="' + bodyW + '" height="' + h + '" fill="' + color + '" opacity="0.5"/>';
  }
  // 日期刻度
  const idxs = [0, Math.floor(n / 2), n - 1];
  for (const i of idxs) {
    svg += '<text x="' + x(i) + '" y="' + (H - 6) + '" text-anchor="middle" font-size="10" fill="var(--faint)">' + candles[i].date.slice(5) + '</text>';
  }
  // 图例
  svg += '<text x="' + (W - padR) + '" y="' + (padT + 10) + '" text-anchor="end" font-size="10" fill="var(--faint)">MA5 橙 · MA10 蓝 · MA20 紫</text>';

  // ---- 交互层：十字光标 + 高亮（初始隐藏） ----
  svg += '<line id="kv-line" stroke="var(--faint)" stroke-width="1" stroke-dasharray="3 3" visibility="hidden"/>';
  svg += '<line id="kh-line" stroke="var(--faint)" stroke-width="1" stroke-dasharray="3 3" visibility="hidden"/>';
  svg += '<rect id="k-hl" fill="var(--accent-weak)" opacity="0.55" visibility="hidden"/>';

  el.innerHTML = '<svg viewBox="0 0 ' + W + ' ' + H + '" style="width:100%;height:auto;display:block;touch-action:none">' + svg + '</svg>';
  window.__lastKlineData = { candles: candles, quote: quote }; // 缓存日K数据（切回时重新渲染恢复交互）

  // 明细浮层（HTML 覆盖层）
  let tip = el.querySelector('.kline-tip');
  if (!tip) { tip = document.createElement('div'); tip.className = 'kline-tip'; tip.style.display = 'none'; el.appendChild(tip); }

  // 交互上下文（几何参数供事件处理复用）
  el._k = {
    svg: el.querySelector('svg'), candles, quote, tip,
    vline: el.querySelector('#kv-line'), hline: el.querySelector('#kh-line'), hl: el.querySelector('#k-hl'),
    W, H, padL, padR, padT, priceH, cw, min, max, n, bodyW, locked: false
  };

  const svgEl = el._k.svg;
  svgEl.addEventListener('mousemove', e => { if (!el._k.locked) klineHover(e, false); });
  svgEl.addEventListener('mouseleave', () => { if (!el._k.locked) klineHide(); });
  svgEl.addEventListener('touchstart', e => { e.preventDefault(); klineHover(e, true); }, { passive: false });
  svgEl.addEventListener('click', () => { el._k.locked = !el._k.locked; if (!el._k.locked) klineHide(); });
}

/* ---- 悬停命中 + 展示 ---- */
function klineHover(e, lock) {
  const k = $('kline-chart')._k;
  if (!k) return;
  // CSS 像素 → viewBox 坐标
  const rect = k.svg.getBoundingClientRect();
  const px = (e.clientX - rect.left) * (k.W / rect.width);
  const py = (e.clientY - rect.top) * (k.H / rect.height);
  const idx = Math.floor((px - k.padL) / k.cw);
  if (idx < 0 || idx >= k.n) { klineHide(); return; }

  const cx = k.padL + idx * k.cw + k.cw / 2;
  // 十字光标 + 高亮
  k.vline.setAttribute('x1', cx); k.vline.setAttribute('x2', cx);
  k.vline.setAttribute('y1', k.padT); k.vline.setAttribute('y2', k.H - 26);
  k.vline.setAttribute('visibility', 'visible');
  k.hline.setAttribute('x1', k.padL); k.hline.setAttribute('x2', k.W - k.padR);
  k.hline.setAttribute('y1', py); k.hline.setAttribute('y2', py);
  k.hline.setAttribute('visibility', 'visible');
  k.hl.setAttribute('x', cx - k.bodyW / 2); k.hl.setAttribute('width', k.bodyW);
  k.hl.setAttribute('y', k.padT); k.hl.setAttribute('height', k.H - 26 - k.padT);
  k.hl.setAttribute('visibility', 'visible');

  // 明细内容
  const c = k.candles[idx];
  const prevClose = idx > 0 ? k.candles[idx - 1].close : (k.quote ? k.quote.prev_close : c.open);
  const chg = c.close - prevClose;
  const chgPct = prevClose ? (chg / prevClose * 100) : 0;
  const lvl = chgLevel(chgPct);
  const up = lvl !== 'down';
  const vol = c.volume >= 10000 ? (c.volume / 10000).toFixed(2) + '万手' : c.volume + '手';
  k.tip.innerHTML =
    '<div class="d">' + esc(c.date) + '</div>' +
    '<div class="row"><span>开 <b>' + c.open.toFixed(2) + '</b></span><span>收 <b>' + c.close.toFixed(2) + '</b></span></div>' +
    '<div class="row"><span>高 <b>' + c.high.toFixed(2) + '</b></span><span>低 <b>' + c.low.toFixed(2) + '</b></span></div>' +
    '<div class="row"><span>量 <b>' + vol + '</b></span>' +
    '<span class="' + lvl + '">' + (up ? '+' : '') + chg.toFixed(2) + ' (' + (up ? '+' : '') + chgPct.toFixed(2) + '%)</span></div>';

  // 浮层定位（CSS 像素，防溢出）
  const cssX = (e.clientX - rect.left);
  const cssY = (e.clientY - rect.top);
  let left = cssX + 14, top = cssY - 70;
  const tipW = 190;
  if (left + tipW > rect.width) left = cssX - tipW - 14;
  if (top < 4) top = cssY + 14;
  k.tip.style.left = left + 'px';
  k.tip.style.top = top + 'px';
  k.tip.style.display = 'block';

  if (lock) { k.locked = true; }
}

function klineHide() {
  const k = $('kline-chart')._k;
  if (!k) return;
  k.vline.setAttribute('visibility', 'hidden');
  k.hline.setAttribute('visibility', 'hidden');
  k.hl.setAttribute('visibility', 'hidden');
  k.tip.style.display = 'none';
}

function renderKlineError() {
  const el = $('kline-chart');
  el.innerHTML = '<div class="degraded" style="margin-top:12px">行情数据加载失败，请稍后重试</div>';
  el._k = null;
}

/* ===== 分时图（日K/分时 切换，懒加载） ===== */
let minuteCache = null; // {code, market, data}

/* 切换日K/分时 */
async function switchKline(mode) {
  const dayBtn = $('tab-day'), minBtn = $('tab-minute');
  if (mode === 'minute') {
    dayBtn.classList.remove('active');
    minBtn.classList.add('active');
    if (!minuteCache) {
      const code = $('stock-head') ? ($('stock-head').querySelector('.code') || {}).textContent : '';
      const market = $('stock-head') ? ($('stock-head').querySelector('.market') || {}).textContent : '';
      if (!code) return;
      $('kline-chart').innerHTML = '<div class="degraded" style="margin-top:12px">加载分时数据…</div>';
      try {
        const data = await apiMinute(code, market);
        if (!data || !data.points || !data.points.length) { $('kline-chart').innerHTML = '<div class="degraded" style="margin-top:12px">暂无分时数据</div>'; return; }
        minuteCache = { code, market, data };
        renderMinute(data);
      } catch (e) {
        $('kline-chart').innerHTML = '<div class="degraded" style="margin-top:12px">分时加载失败，请稍后重试</div>';
      }
    } else {
      renderMinute(minuteCache.data);
    }
  } else {
    minBtn.classList.remove('active');
    dayBtn.classList.add('active');
    if (window.__lastKlineData) { // 用缓存数据重新渲染（恢复交互绑定）
      renderKline(window.__lastKlineData.candles, window.__lastKlineData.quote);
    }
  }
}

/* 分时 SVG：价格线 + 均价线 + 昨收虚线（涨红跌绿） */
function renderMinute(data) {
  const pts = data.points;
  const n = pts.length;
  const W = 860, H = 340, padL = 58, padR = 12, padT = 18;
  const plotW = W - padL - padR;
  const pre = data.pre_close || pts[0].price;
  const cw = plotW / n;
  const x = i => padL + i * cw + cw / 2;

  let min = Infinity, max = -Infinity;
  for (const p of pts) {
    if (p.price < min) min = p.price;
    if (p.price > max) max = p.price;
    if (p.avg_price > 0 && p.avg_price < min) min = p.avg_price;
    if (p.avg_price > 0 && p.avg_price > max) max = p.avg_price;
  }
  if (pre < min) min = pre;
  if (pre > max) max = pre;
  const pad = (max - min) * 0.08 || 1;
  min -= pad; max += pad;
  const y = v => padT + (max - v) / (max - min) * (H - 40 - padT);

  const line = (get, color, w) => {
    const pts2 = [];
    for (let i = 0; i < n; i++) {
      const v = get(pts[i]);
      if (v > 0) pts2.push(x(i).toFixed(1) + ',' + y(v).toFixed(1));
    }
    return '<polyline points="' + pts2.join(' ') + '" fill="none" stroke="' + color + '" stroke-width="' + (w || 1.5) + '"/>';
  };
  const up = pts[n - 1].price >= pre;
  const priceColor = up ? 'var(--bull)' : 'var(--bear)';

  let svg = '<svg width="100%" viewBox="0 0 ' + W + ' ' + H + '" style="display:block">';
  // 网格 + 刻度
  const gl = 4;
  for (let i = 0; i <= gl; i++) {
    const v = min + (max - min) * i / gl, yy = y(v);
    svg += '<line x1="' + padL + '" y1="' + yy + '" x2="' + (W - padR) + '" y2="' + yy + '" stroke="var(--border)" stroke-width="1"/>';
    svg += '<text x="' + (padL - 6) + '" y="' + (yy + 4) + '" text-anchor="end" font-size="10" fill="var(--faint)">' + v.toFixed(2) + '</text>';
  }
  // 昨收虚线
  const py = y(pre);
  svg += '<line x1="' + padL + '" y1="' + py + '" x2="' + (W - padR) + '" y2="' + py + '" stroke="var(--neutral)" stroke-width="1" stroke-dasharray="4,3"/>';
  svg += '<text x="' + (padL + 4) + '" y="' + (py - 4) + '" font-size="10" fill="var(--neutral)">昨收 ' + pre.toFixed(2) + '</text>';
  // 均价线
  svg += line(p => p.avg_price, '#f7b731', 1.3);
  // 价格线（渐变填充面积可选，简单折线）
  svg += line(p => p.price, priceColor, 1.8);
  // 底部时间刻度
  const times = [pts[0].time, '10:30', '11:30/13:00', '14:00', pts[n - 1].time];
  times.forEach((t, i) => {
    const tx = padL + plotW * i / (times.length - 1);
    svg += '<text x="' + tx + '" y="' + (H - 14) + '" text-anchor="' + (i === 0 ? 'start' : i === times.length - 1 ? 'end' : 'middle') + '" font-size="10" fill="var(--faint)">' + t + '</text>';
  });
  // 右上角涨跌
  const chg = pts[n - 1].price - pre;
  const chgPct = pre ? (chg / pre * 100) : 0;
  svg += '<text x="' + (W - padR) + '" y="' + (padT + 6) + '" text-anchor="end" font-size="12" font-weight="700" fill="' + priceColor + '">' +
    (up ? '+' : '') + chg.toFixed(2) + '  (' + (up ? '+' : '') + chgPct.toFixed(2) + '%)</text>';

  // ---- 交互层：十字光标 + 高亮（初始隐藏，id 加 mv- 前缀避免与日K冲突） ----
  svg += '<line id="mv-line" stroke="var(--faint)" stroke-width="1" stroke-dasharray="3 3" visibility="hidden"/>';
  svg += '<line id="mh-line" stroke="var(--faint)" stroke-width="1" stroke-dasharray="3 3" visibility="hidden"/>';
  svg += '<rect id="m-hl" fill="var(--accent-weak)" opacity="0.4" visibility="hidden"/>';
  svg += '</svg>';

  const el = $('kline-chart');
  el.innerHTML = svg;

  // 明细浮层（复用 .kline-tip 样式）
  let tip = el.querySelector('.kline-tip');
  if (!tip) { tip = document.createElement('div'); tip.className = 'kline-tip'; tip.style.display = 'none'; el.appendChild(tip); }
  el._m = {
    svg: el.querySelector('svg'), pts, tip, pre,
    vline: el.querySelector('#mv-line'), hline: el.querySelector('#mh-line'), hl: el.querySelector('#m-hl'),
    W, H, padL, padR, padT, cw, min, max, n, locked: false
  };
  const svgEl = el._m.svg;
  svgEl.addEventListener('mousemove', e => { if (!el._m.locked) minuteHover(e, false); });
  svgEl.addEventListener('mouseleave', () => { if (!el._m.locked) minuteHide(); });
  svgEl.addEventListener('touchstart', e => { e.preventDefault(); minuteHover(e, true); }, { passive: false });
  svgEl.addEventListener('click', () => { el._m.locked = !el._m.locked; if (!el._m.locked) minuteHide(); });
}

/* ---- 分时悬停：十字光标 + 明细（时间/价格/涨跌幅/均价/成交量） ---- */
function minuteHover(e, lock) {
  const m = $('kline-chart')._m;
  if (!m) return;
  const rect = m.svg.getBoundingClientRect();
  const px = (e.clientX - rect.left) * (m.W / rect.width);
  const py = (e.clientY - rect.top) * (m.H / rect.height);
  const idx = Math.round((px - m.padL) / m.cw);
  if (idx < 0 || idx >= m.n) { minuteHide(); return; }

  const p = m.pts[idx];
  const cx = m.padL + idx * m.cw + m.cw / 2;
  const cy = m.padT + (m.max - p.price) / (m.max - m.min) * (m.H - 40 - m.padT);

  // 十字光标 + 高亮列
  m.vline.setAttribute('x1', cx); m.vline.setAttribute('x2', cx);
  m.vline.setAttribute('y1', m.padT); m.vline.setAttribute('y2', m.H - 30);
  m.vline.setAttribute('visibility', 'visible');
  m.hline.setAttribute('x1', m.padL); m.hline.setAttribute('x2', m.W - m.padR);
  m.hline.setAttribute('y1', py); m.hline.setAttribute('y2', py);
  m.hline.setAttribute('visibility', 'visible');
  m.hl.setAttribute('x', cx - m.cw / 2); m.hl.setAttribute('width', m.cw);
  m.hl.setAttribute('y', m.padT); m.hl.setAttribute('height', m.H - 30 - m.padT);
  m.hl.setAttribute('visibility', 'visible');

  // 明细浮层
  const chg = p.price - m.pre;
  const chgPct = m.pre ? (chg / m.pre * 100) : 0;
  const up = chg >= 0;
  const color = up ? 'var(--bull)' : 'var(--bear)';
  m.tip.innerHTML =
    '<div class="d">' + esc(p.time) + '</div>' +
    '<div class="row"><span>价 <b style="color:' + color + '">' + p.price.toFixed(2) + '</b></span>' +
    '<span style="color:' + color + '">' + (up ? '+' : '') + chg.toFixed(2) + ' (' + (up ? '+' : '') + chgPct.toFixed(2) + '%)</span></div>' +
    '<div class="row"><span>均价 <b>' + (p.avg_price > 0 ? p.avg_price.toFixed(2) : '—') + '</b></span>' +
    '<span>量 <b>' + (p.volume >= 10000 ? (p.volume / 10000).toFixed(1) + '万手' : p.volume + '手') + '</b></span></div>';
  m.tip.style.display = 'block';

  // 浮层定位（防溢出）
  const cssX = (e.clientX - rect.left);
  const cssY = (e.clientY - rect.top);
  let left = cssX + 14, top = cssY - 70;
  if (left + 170 > rect.width) left = cssX - 184;
  if (top < 4) top = 4;
  m.tip.style.left = left + 'px';
  m.tip.style.top = top + 'px';
}

function minuteHide() {
  const m = $('kline-chart')._m;
  if (!m) return;
  m.vline.setAttribute('visibility', 'hidden');
  m.hline.setAttribute('visibility', 'hidden');
  m.hl.setAttribute('visibility', 'hidden');
  m.tip.style.display = 'none';
}
