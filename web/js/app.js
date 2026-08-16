/* app.js —— 应用入口：视图切换 / 搜索联想 / SSE 事件分发 / 分析流式状态 */
let analysisText = '';
let renderTimer = null;

/* ---- 视图切换 ---- */
function showDetail() {
  $('view-search').classList.add('hidden');
  $('view-detail').classList.remove('hidden');
  window.scrollTo({ top: 0 });
}
function showSearch() {
  $('view-detail').classList.add('hidden');
  $('view-search').classList.remove('hidden');
}

/* ---- 搜索联想（防抖调用识别接口） ---- */
let suggestTimer = null;
$('stock-input').addEventListener('input', () => {
  const q = $('stock-input').value.trim();
  clearTimeout(suggestTimer);
  hideSuggest();
  if (q.length < 1) return;
  suggestTimer = setTimeout(async () => {
    try {
      const info = await apiResolve(q);
      if (!info || !info.code) return;
      const box = $('suggest');
      box.classList.remove('hidden');
      box.innerHTML = '<div class="suggest-item" onclick="useSuggestion()">' +
        '<span class="name">' + esc(info.name) + '</span>' +
        '<span class="code">' + esc(info.code) + '</span>' +
        '<span class="market">' + esc(info.market) + '</span></div>';
      box._stock = info;
    } catch (e) { /* 忽略联想失败 */ }
  }, 300);
});
function hideSuggest() { $('suggest').classList.add('hidden'); }
function useSuggestion() {
  const info = $('suggest')._stock;
  if (!info) return;
  $('stock-input').value = info.name;
  hideSuggest();
  doSearch();
}

/* ---- 查询（SSE 全链路） ---- */
async function doSearch() {
  const stock = $('stock-input').value.trim();
  if (!stock) { $('stock-input').focus(); return; }
  const btn = $('search-btn');
  btn.disabled = true;
  hideSuggest();

  $('stock-head').innerHTML = '';
  $('quote-card').classList.add('hidden');
  $('quote-body').innerHTML = '';
  $('kline-chart').innerHTML = '';
  $('news-card').classList.add('hidden');
  $('news-body').innerHTML = '';
  $('sentiment-body').innerHTML = '<span style="color:var(--faint)">正在分析…</span>';
  $('analysis').innerHTML = '';
  analysisText = '';
  hideError();
  $('cursor').classList.remove('hidden');
  showDetail();

  try {
    const resp = await apiAsk(stock);
    if (!resp.ok) {
      const j = await resp.json().catch(() => ({}));
      showError(j.error || ('HTTP ' + resp.status));
      return;
    }
    await readSSE(resp, handleEvent);
    flushAnalysis();
  } catch (e) {
    showError('网络异常：' + e.message);
  } finally {
    btn.disabled = false;
    $('cursor').classList.add('hidden');
  }
}

/* ---- SSE 事件分发 ---- */
function handleEvent(event, data) {
  let d = {};
  try { d = data ? JSON.parse(data) : {}; } catch (e) {}
  switch (event) {
    case 'stock':
      $('stock-head').innerHTML =
        '<span class="name">' + esc(d.name || '') + '</span>' +
        '<span class="code">' + esc(d.code || '') + '</span>' +
        '<span class="market">' + esc(d.market || '') + '</span>' +
        '<span class="fresh">实时检索</span>';
      fetchKline(d.code, d.market); // 异步拉行情，不阻塞 SSE 流
      fetchNews(d.name);            // 异步拉相关资讯（辅助，失败静默）
      break;
    case 'sentiment': renderSentiment(d); break;
    case 'delta': appendDelta(d.text || ''); break;
    case 'error': showError(d.message || '发生错误'); break;
    case 'done': break;
  }
}

/* ---- 行情（报价 + 日K，异步） ---- */
async function fetchKline(code, market) {
  try {
    const data = await apiKline(code, market, 60);
    if (!data || !data.quote) { renderKlineError(); return; }
    renderQuote(data.quote);
    renderKline(data.candles || [], data.quote); // quote 用于首根涨跌幅计算
  } catch (e) { renderKlineError(); }
}

/* ---- 相关资讯（辅助，失败静默隐藏） ---- */
async function fetchNews(name) {
  try {
    const items = await apiNews(name, 5);
    if (items && items.length) renderNews(items);
  } catch (e) { /* 静默降级 */ }
}

/* ---- 分析流式（markdown 防抖渲染） ---- */
function appendDelta(text) {
  analysisText += text;
  if (renderTimer) return;
  renderTimer = setTimeout(() => { renderTimer = null; $('analysis').innerHTML = renderMarkdown(analysisText); }, 80);
}
function flushAnalysis() {
  if (renderTimer) { clearTimeout(renderTimer); renderTimer = null; }
  $('analysis').innerHTML = renderMarkdown(analysisText);
}

/* ---- 初始化 ---- */
$('stock-input').addEventListener('keydown', e => {
  if (e.key === 'Enter') { hideSuggest(); doSearch(); }
});
window.addEventListener('load', () => $('stock-input').focus());
