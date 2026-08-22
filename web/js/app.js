/* app.js —— 应用入口：视图切换 / 搜索联想 / SSE 事件分发 / 分析流式状态 */
let analysisText = '';
let renderTimer = null;
let gotFundamental = false; // 本次分析是否收到基本面评分
let searchId = 0;            // 搜索序号（doSearch 重入保护）

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
  if (renderTimer) { clearTimeout(renderTimer); renderTimer = null; }
  const myId = ++searchId; // 本次搜索 token：后续续体校验仍是当前

  $('stock-head').innerHTML =
    '<span class="name">正在查询 ' + esc(stock) + '…</span>' +
    '<span class="fresh" style="color:var(--faint)">识别中</span>';
  $('quote-card').classList.add('hidden');
  $('quote-body').innerHTML = '';
  $('kline-chart').innerHTML = '';
  minuteCache = null; // 分时缓存重置（新股票重新加载）
  window.__lastKlineData = null;
  stopMinuteRefresh(); // 停止分时轮询
  if ($('tab-day')) { $('tab-day').classList.add('active'); $('tab-minute').classList.remove('active'); }
  $('news-card').classList.add('hidden');
  $('news-body').innerHTML = '';
  $('finance-card').classList.add('hidden');   // 财报卡片：切股重置
  $('finance-table').innerHTML = '';
  $('finance-analysis').innerHTML = '';
  $('video-card').classList.add('hidden');      // 视频卡片：切股重置
  $('video-body').innerHTML = '';
  $('chat-card').classList.add('hidden');   // 二期：切股重置对话区（会话由服务端按 code 隔离）
  $('chat-msgs').innerHTML = '';
  gotFundamental = false;
  $('chain-card').classList.add('hidden');
  $('chain-body').innerHTML = '';
  $('chain-companies').innerHTML = '';
  $('fundamental-card').classList.add('hidden');
  $('fundamental-score').innerHTML = '<span class="fin-loading">正在评分…</span>';
  $('fundamental-analysis').innerHTML = '<span id="cursor" class="blink"></span>';
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
    await readSSE(resp, (event, data) => handleEvent(event, data, myId)); // 传本次搜索 token（防旧流污染）
    flushAnalysis();
  } catch (e) {
    showError('网络异常：' + e.message);
  } finally {
    btn.disabled = false;
    const c = $('cursor');
    if (c) c.classList.add('hidden');
    if (myId !== searchId) return; // 已被新搜索取代，丢弃
  }
}

/* ---- SSE 事件分发 ---- */
function handleEvent(event, data, myId) {
  if (myId === undefined) myId = searchId; // 兼容直接调用
  let d = {};
  try { d = data ? JSON.parse(data) : {}; } catch (e) {}
  switch (event) {
    case 'stock':
      if (myId !== searchId) break;
      $('stock-head').innerHTML =
        '<span class="name">' + esc(d.name || '') + '</span>' +
        '<span class="code">' + esc(d.code || '') + '</span>' +
        '<span class="market">' + esc(d.market || '') + '</span>' +
        '<span class="fresh">实时检索</span>';
      fetchKline(d.code, d.market); // 异步拉行情，不阻塞 SSE 流
      fetchNews(d.name);            // 异步拉相关资讯（辅助，失败静默）
      loadFinance(d.code, d.market); // 财报解析：指标 + AI 解析（东财双源）
      loadChain(d.code, d.market, searchId); // 产业链图谱（AI 生成，带重入保护）
      loadVideo(d.name);             // 相关视频（B站，封面卡片横滑）

      resetChat({ code: d.code, market: d.market, name: d.name }); // 二期：绑定看山对话
      break;
    case 'fundamental': if (myId !== searchId) break; gotFundamental = true; renderFundamental(d); break;
    case 'delta': if (myId !== searchId) break; appendDelta(d.text || ''); break;
    case 'done':
      if (myId !== searchId) break;
      if (!gotFundamental) {
        $('fundamental-card').classList.remove('hidden');
        $('fundamental-score').innerHTML = '<span style="color:var(--faint)">评分暂不可用（数据源异常）</span>';
      }
      break;
    case 'error': if (myId === searchId) showError(d.message || '发生错误'); break;
    case 'done': break;
  }
}

/* ---- 行情（报价 + 日K，异步） ---- */
async function fetchKline(code, market) {
  const myId = searchId;
  try {
    const data = await apiKline(code, market, 60);
    if (myId !== searchId) return; // 已切股/新搜索
    if (!data || !data.quote) { renderKlineError(); return; }
    renderQuote(data.quote);
    renderKline(data.candles || [], data.quote); // quote 用于首根涨跌幅计算
  } catch (e) { if (myId === searchId) renderKlineError(); }
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
  renderTimer = setTimeout(() => { renderTimer = null; $('fundamental-analysis').innerHTML = renderMarkdown(analysisText); }, 80);
}
function flushAnalysis() {
  if (renderTimer) { clearTimeout(renderTimer); renderTimer = null; }
  $('fundamental-analysis').innerHTML = renderMarkdown(analysisText);
}

/* ---- 初始化 ---- */
$('stock-input').addEventListener('keydown', e => {
  if (e.key === 'Enter') { hideSuggest(); doSearch(); }
});
window.addEventListener('load', () => $('stock-input').focus());

/* ---- 首页热门加载：知乎热榜（主）+ 股票/板块/暴跌（辅助，失败友好降级） ---- */
async function loadHomeHot() {
  // 股票/板块（辅助数据，限流时友好降级 + 重试）
  const stocks = await apiHot('stock', 10).catch(() => null);
  const sectors = await apiHot('sector', 8).catch(() => null);
  const falls = await apiHot('sector_fall', 8).catch(() => null);
  if (stocks && stocks.length) { renderTicker(stocks); renderHotStocks(stocks); }
  else { renderTicker([]); showHotDegraded('stock'); }
  if (sectors && sectors.length) { renderHotSectors('hot-sectors', 'hot-sectors-card', sectors); }
  else { showHotDegraded('sector'); }
  if (falls && falls.length) { renderHotSectors('hot-fall', 'hot-fall-card', falls); }
  else { showHotDegraded('sector_fall'); }
}

function retryHot() {
  const label = $('hot-stocks-label'), back = $('hot-stocks-back');
  if (label) label.textContent = ''; if (back) back.classList.add('hidden');
  loadHomeHot();
}

/* 点击热门股票 → 直接查询 */
function hotSearch(name) {
  $('stock-input').value = name;
  hideSuggest();
  doSearch();
}

/* 首页初始化：热门 + 输入框聚焦 */
window.addEventListener('load', () => {
  $('stock-input').focus();
  loadHomeHot();
});

/* ---- 板块点击 → 成分股（复用热门股票卡片） ---- */
async function hotSectorClick(code, name) {
  $('hot-stocks').innerHTML = '<span class="hot-loading">加载板块成分股…</span>';
  $('hot-stocks-card').classList.remove('hidden');
  const items = await apiHot('sector_stock', 10, code).catch(() => null);
  if (items && items.length) {
    renderHotStocks(items);
    setHotContext(true, name);
  } else {
    $('hot-stocks').innerHTML = '<span class="hot-loading">板块成分加载失败，请稍后重试</span>';
  }
}

function backToHot() {
  setHotContext(false);
  loadHomeHot(); // 重新加载热门榜
}

function setHotContext(isSector, name) {
  const label = $('hot-stocks-label');
  const back = $('hot-stocks-back');
  if (isSector) {
    label.textContent = ' · 板块成分：' + (name || '');
    back.classList.remove('hidden');
  } else {
    label.textContent = '';
    back.classList.add('hidden');
  }
}

