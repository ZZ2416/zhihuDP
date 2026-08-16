/* kline.js —— K线图渲染：SVG 蜡烛图（MA5/10/20 + 成交量 + 坐标轴） */
function renderKline(candles) {
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

  el.innerHTML = '<svg viewBox="0 0 ' + W + ' ' + H + '" style="width:100%;height:auto;display:block">' + svg + '</svg>';
}

function renderKlineError() {
  $('kline-chart').innerHTML = '<div class="degraded" style="margin-top:12px">行情数据加载失败，请稍后重试</div>';
}
