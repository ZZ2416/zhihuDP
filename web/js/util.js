/* util.js —— 通用工具（DOM 简写 / HTML 转义 / 涨幅四档分级） */
function $(id) { return document.getElementById(id); }

function esc(s) {
  return String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

/* 涨幅四档分级（A股色阶惯例）：
 * <0 跌 → down（绿）；[0,2) 微涨 → up1（黄）；[2,4) 中涨 → up2（橙）；≥4 大涨 → up3（红）
 * 返回 CSS 类名，配合 style.css 的 .up1/.up2/.up3/.down 使用 */
function chgLevel(pct) {
  const p = Number(pct) || 0;
  if (p < 0) return 'down';
  if (p < 2) return 'up1';
  if (p < 4) return 'up2';
  return 'up3';
}
