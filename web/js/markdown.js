/* markdown.js —— AI 分析文本的轻量 markdown 渲染（标题/加粗/列表/链接/分割线） */
function mdInline(s) {
  s = s.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
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
