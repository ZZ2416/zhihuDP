/* markdown.js —— AI 文本的轻量 markdown 渲染（标题/加粗/列表/链接/分割线）
 * 安全：先整体 HTML 转义再套 markdown 变换（防存储型 XSS）；链接协议白名单 http/https */
function mdInline(raw) {
  let s = esc(raw); // 先转义，markdown 标记（** [] # - 等）不受影响
  s = s.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (m, txt, href) => {
    const u = href.trim();
    if (!/^https?:\/\//i.test(u)) return txt; // 非 http(s) 链接按纯文本处理
    return '<a href="' + u + '" target="_blank" rel="noopener">' + txt + '</a>';
  });
  s = s.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  s = s.replace(/`([^`]+)`/g, '<code>$1</code>');
  return s;
}

function renderMarkdown(text) {
  let html = '', listOpen = false;
  const closeList = () => { if (listOpen) { html += '</ul>'; listOpen = false; } };
  for (const raw of text.split('\n')) {
    const line = raw.trim();
    if (!line) { closeList(); continue; }
    if (/^-{3,}/.test(line)) { closeList(); html += '<hr>'; continue; }
    let m = line.match(/^(#{1,4})\s+(.*)/);
    if (m) { closeList(); const lvl = m[1].length; html += '<h' + lvl + '>' + mdInline(m[2]) + '</h' + lvl + '>'; continue; }
    if (/^[-*•]\s+/.test(line)) { if (!listOpen) { html += '<ul>'; listOpen = true; } html += '<li>' + mdInline(line.replace(/^[-*•]\s+/, '')) + '</li>'; continue; }
    m = line.match(/^\d+[.、]\s+(.*)/);
    if (m) { if (!listOpen) { html += '<ul>'; listOpen = true; } html += '<li>' + mdInline(m[1]) + '</li>'; continue; }
    closeList();
    html += '<p>' + mdInline(line) + '</p>';
  }
  closeList();
  return html;
}
