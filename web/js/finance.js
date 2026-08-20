/* finance.js —— 财报解析：指标表格（5 年年报）+ AI 解析（SSE 流式，自动触发） */
let renderMdTimer = null;
/* 财务指标行定义：label + 取值函数 + 百分比格式 */
const FIN_ROWS = [
  ['营业总收入（亿）', it => it.revenue, 1],
  ['营收同比 %', it => it.revenue_yoy, 1],
  ['归母净利润（亿）', it => it.net_profit, 1],
  ['净利同比 %', it => it.net_profit_yoy, 1],
  ['每股收益（元）', it => it.eps, 2],
  ['ROE（加权）%', it => it.roe, 2],
  ['毛利率 %', it => it.gross_margin, 2],
  ['净利率 %', it => it.net_margin, 2],
  ['资产负债率 %', it => it.debt_ratio, 2],
  ['经营现金流/营收', it => it.cash_flow_to_rev, 2],
];

/* 加载财务数据 → 渲染表格 → 自动 AI 解析 */
async function loadFinance(code, market) {
  const card = $('finance-card');
  if (!card) return;
  card.classList.remove('hidden');
  $('finance-table').innerHTML = '<span class="fin-loading">加载财务数据…</span>';
  $('finance-analysis').innerHTML = '<span class="fin-loading">正在解析财报…</span>';
  try {
    const resp = await apiFinance(code, market);
    if (!resp.ok) {
      const j = await resp.json().catch(() => ({}));
      throw new Error(j.error || ('HTTP ' + resp.status));
    }
    const res = await resp.json();
    if (!res.indicators || !res.indicators.length) throw new Error('暂无财务数据');
    renderFinanceTable(res.indicators);
    if (res.name) {
      const head = $('stock-head');
      if (head && !head.textContent.includes(res.name)) { /* 名称已由 stock 事件渲染，无需处理 */ }
    }
    analyzeFinance(code, market); // 自动触发 AI 解析
  } catch (e) {
    $('finance-table').innerHTML =
      '<div class="degraded" style="margin-top:4px">财务数据暂不可用：' + esc(e.message) +
      ' <a style="cursor:pointer" onclick="retryFinance(\'' + esc(code) + '\',\'' + esc(market || '') + '\')">重试</a></div>';
    $('finance-analysis').innerHTML = '';
  }
}

function retryFinance(code, market) { loadFinance(code, market); }

/* 指标表格：行=指标，列=报告期（最新在前） */
function renderFinanceTable(indicators) {
  const head = '<thead><tr><th class="fin-label">指标</th>' +
    indicators.map(it => '<th>' + esc(it.report_date) + '</th>').join('') + '</tr></thead>';
  let body = '';
  for (const [label, get, dec] of FIN_ROWS) {
    let cells = '';
    for (const it of indicators) {
      const v = get(it);
      cells += '<td>' + (typeof v === 'number' ? v.toFixed(dec) : '—') + '</td>';
    }
    body += '<tr><td class="fin-label">' + label + '</td>' + cells + '</tr>';
  }
  $('finance-table').innerHTML =
    '<div class="fin-table-wrap"><table class="fin-table">' + head + '<tbody>' + body + '</tbody></table></div>';
}

/* AI 解析（SSE 流式） */
async function analyzeFinance(code, market) {
  const box = $('finance-analysis');
  if (!box) return;
  box.innerHTML = '<span class="fin-loading">正在解析财报…</span>';
  let text = '';
  try {
    const resp = await apiFinanceAnalyze(code, market);
    if (!resp.ok) {
      const j = await resp.json().catch(() => ({}));
      throw new Error(j.error || ('HTTP ' + resp.status));
    }
    box.innerHTML = '<div class="fin-ai"></div>';
    const el = box.querySelector('.fin-ai');
    await readSSE(resp, (event, data) => {
      let d = {};
      try { d = data ? JSON.parse(data) : {}; } catch (e) {}
      if (event === 'delta') {
        text += d.text || '';
        clearTimeout(renderMdTimer); // 防抖：80ms 内合并增量，避免每 delta 全量重渲染 O(n²)
        renderMdTimer = setTimeout(() => { el.innerHTML = renderMarkdown(text); }, 80);
      } else if (event === 'error') {
        el.innerHTML = '<span style="color:var(--err-text)">财报解析失败：' + esc(d.message || '请稍后重试') + '</span>';
      }
    });
  } catch (e) {
    box.innerHTML = '<span style="color:var(--err-text)">财报解析失败：' + esc(e.message) + '</span>';
  }
}
