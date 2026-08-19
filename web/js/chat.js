/* chat.js —— 二期「与看山对话」：独立对话区（K 线面板下方）
 * 每个股票独立会话（服务端按 stock 隔离）；发送追问走 POST /api/chat SSE 流式。 */
let chatStock = null; // {code, market, name} 当前对话绑定的股票
let chatBusy = false;

/* 切股/新查询时重置对话区（会话本身由服务端按 code 隔离） */
function resetChat(stock) {
  chatStock = stock || null;
  chatBusy = false;
  $('chat-msgs').innerHTML = '';
  $('chat-card').classList.remove('hidden');
  $('chat-input').value = '';
}

/* 追加一条消息；streaming=true 返回内容容器供增量追加 */
function appendChatMsg(role, text, streaming) {
  const box = $('chat-msgs');
  const wrap = document.createElement('div');
  wrap.className = 'chat-msg ' + (role === 'user' ? 'user' : 'assistant');
  if (role === 'assistant') {
    const av = document.createElement('div');
    av.className = 'chat-bubble-avatar chat-avatar-hdw';
    av.textContent = '黑';
    wrap.appendChild(av);
  }
  const bubble = document.createElement('div');
  bubble.className = 'chat-bubble';
  const content = document.createElement('div');
  content.className = 'chat-content';
  if (text) content.textContent = text;
  bubble.appendChild(content);
  wrap.appendChild(bubble);
  box.appendChild(wrap);
  box.scrollTop = box.scrollHeight;
  return streaming ? content : wrap;
}

/* 增量 delta：追加纯文本（done 后统一 markdown 渲染） */
function appendChatDelta(contentEl, delta) {
  if (!contentEl) return;
  contentEl.textContent += delta;
  $('chat-msgs').scrollTop = $('chat-msgs').scrollHeight;
}

/* 结束：markdown 渲染最终回复 */
function finalizeChatMsg(contentEl) {
  if (!contentEl) return;
  contentEl.innerHTML = renderMarkdown(contentEl.textContent || '');
}

/* 发送追问 */
async function sendChat() {
  const input = $('chat-input');
  const msg = (input.value || '').trim();
  if (!msg || chatBusy || !chatStock) return;
  appendChatMsg('user', msg);
  input.value = '';
  chatBusy = true;
  $('chat-send').disabled = true;

  const thinking = appendChatMsg('assistant', '看山正在思考…', true);
  try {
    const resp = await apiChat(chatStock.code, chatStock.market, msg);
    if (!resp.ok) {
      const j = await resp.json().catch(() => ({}));
      throw new Error(j.error || ('HTTP ' + resp.status));
    }
    thinking.textContent = '';
    await readSSE(resp, (event, data) => {
      let d = {};
      try { d = data ? JSON.parse(data) : {}; } catch (e) {}
      if (event === 'delta') appendChatDelta(thinking, d.text || '');
      else if (event === 'error') {
        thinking.textContent = '😥 ' + (d.message || '对话出错了，请稍后重试。');
      }
    });
    finalizeChatMsg(thinking);
  } catch (e) {
    thinking.textContent = '😥 网络异常：' + e.message + '（点击「发送」可重试）';
  } finally {
    chatBusy = false;
    $('chat-send').disabled = false;
    $('chat-input').focus();
  }
}

/* 清空会话（服务端删除该股票会话，重新开始） */
async function clearChat() {
  if (!chatStock) return;
  try { await apiChatReset(chatStock.code); } catch (e) { /* 服务端会话删除失败不阻塞 */ }
  $('chat-msgs').innerHTML = '';
  $('chat-input').value = '';
}

/* 输入框回车发送 */
$('chat-input').addEventListener('keydown', e => {
  if (e.key === 'Enter') { e.preventDefault(); sendChat(); }
});
