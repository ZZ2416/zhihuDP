/* chain.js —— 产业链图谱：AI 生成 JSON → 三列 SVG 渲染 + 节点交互（点击看同类型厂商） */
let chainNodes = null; // 缓存当前产业链（切换股票时重置）

async function loadChain(code, market) {
  const card = $('chain-card');
  if (!card) return;
  card.classList.remove('hidden');
  $('chain-body').innerHTML = '<span class="fin-loading">AI 正在绘制产业链…</span>';
  $('chain-companies').innerHTML = '';
  chainNodes = null;
  try {
    const resp = await apiChain(code, market);
    if (!resp.ok) {
      const j = await resp.json().catch(() => ({}));
      throw new Error(j.error || ('HTTP ' + resp.status));
    }
    const res = await resp.json();
    if (!res.nodes || !res.nodes.length) throw new Error('产业链生成失败');
    chainNodes = res;
    renderChain(res);
  } catch (e) {
    $('chain-body').innerHTML = '<div class="degraded" style="margin-top:6px">产业链生成失败：' + esc(e.message) +
      ' <a style="cursor:pointer" onclick="retryChain()">重试</a></div>';
  }
}

function retryChain() {
  if (curStock) loadChain(curStock.code, curStock.market);
}

/* 三列 SVG 渲染：上游 | 中游 | 下游 */
function renderChain(res) {
  const stages = ['上游', '中游', '下游'];
  const byStage = {};
  for (const s of stages) byStage[s] = res.nodes.filter(n => n.stage === s);
  const colW = 210, gapX = 40, nodeH = 54, gapY = 22;
  const W = stages.length * colW + gapX * 2 + 60;
  // 每列最高节点数
  const maxPerCol = Math.max(...stages.map(s => byStage[s].length), 1);
  const H = maxPerCol * (nodeH + gapY) + gapY * 2 + 60;
  const colX = s => 30 + stages.indexOf(s) * (colW + gapX) + colW / 2;

  const yPos = {};
  for (const s of stages) {
    const list = byStage[s];
    const colH = list.length * (nodeH + gapY);
    const top = (H - colH) / 2;
    list.forEach((n, i) => { yPos[n.id] = top + i * (nodeH + gapY); });
  }

  let svg = '<svg viewBox="0 0 ' + W + ' ' + H + '" style="width:100%;height:auto;display:block;min-width:720px">';
  // 列标题
  stages.forEach(s => {
    svg += '<text x="' + colX(s) + '" y="26" text-anchor="middle" font-size="15" font-weight="700" fill="var(--sub)">' + s + '</text>';
  });
  // 连线（贝塞尔）
  svg += '<defs><marker id="chain-arrow" markerWidth="8" markerHeight="8" refX="7" refY="3" orient="auto"><path d="M0,0 L7,3 L0,6 Z" fill="var(--faint)"/></marker></defs>';
  for (const e of res.edges) {
    if (yPos[e.from] === undefined || yPos[e.to] === undefined) continue;
    const x1 = colX(stageOf(res, e.from)), y1 = yPos[e.from] + nodeH / 2;
    const x2 = colX(stageOf(res, e.to)), y2 = yPos[e.to] + nodeH / 2;
    const mx = (x1 + x2) / 2;
    svg += '<path d="M' + x1 + ',' + y1 + ' C' + mx + ',' + y1 + ' ' + mx + ',' + y2 + ' ' + (x2 - 8) + ',' + y2 + '"' +
      ' fill="none" stroke="var(--faint)" stroke-width="1.4" marker-end="url(#chain-arrow)"/>';
  }
  // 节点
  for (const n of res.nodes) {
    const x = colX(n.stage), y = yPos[n.id];
    svg += '<g class="chain-node" data-id="' + n.id + '" transform="translate(' + (x - colW / 2 + 14) + ',' + y + ')">' +
      '<rect x="0" y="0" width="' + (colW - 28) + '" height="' + nodeH + '" rx="9" fill="var(--card)" stroke="var(--border)" stroke-width="1.2"/>' +
      '<text x="' + (colW - 28) / 2 + '" y="22" text-anchor="middle" font-size="13" font-weight="700" fill="var(--text)">' + esc(n.name) + '</text>' +
      '<text x="' + (colW - 28) / 2 + '" y="40" text-anchor="middle" font-size="11" fill="var(--faint)">' + esc(n.desc || '') + '</text>' +
      '</g>';
  }
  svg += '</svg>';
  $('chain-body').innerHTML =
    '<div class="chain-scroll"><div class="chain-fig">' + svg +
    '<div class="chain-legend">💡 点击环节节点查看同类型厂商 · AI 生成仅供参考</div></div></div>';

  // 节点点击交互
  document.querySelectorAll('#chain-body .chain-node').forEach(g => {
    g.addEventListener('click', () => showChainCompanies(g.getAttribute('data-id')));
  });
  // 默认选中第一个有厂商的节点
  const first = res.nodes.find(n => res.companies && res.companies[n.id] && res.companies[n.id].length);
  if (first) showChainCompanies(first.id);
}

function stageOf(res, id) {
  const n = res.nodes.find(x => x.id === id);
  return n ? n.stage : '中游';
}

/* 节点 → 同类型厂商面板 */
function showChainCompanies(nodeId) {
  const box = $('chain-companies');
  if (!box || !chainNodes) return;
  document.querySelectorAll('#chain-body .chain-node').forEach(g => {
    g.classList.toggle('active', g.getAttribute('data-id') === nodeId);
  });
  const node = chainNodes.nodes.find(n => n.id === nodeId);
  if (!node) return;
  const cs = (chainNodes.companies && chainNodes.companies[nodeId]) || [];
  if (!cs.length) {
    box.innerHTML = '<div class="chain-node-title">' + esc(node.name) + '</div>' +
      '<span class="fin-loading">该环节暂无可用厂商（AI 生成仅供参考）</span>';
    return;
  }
  let html = '<div class="chain-node-title">' + esc(node.name) + ' · 同类型厂商</div><div class="chain-company-list">';
  for (const c of cs) {
    html += '<span class="chain-company" onclick="chainSearch(\'' + esc(c.code) + '\')" title="查看 ' + esc(c.name) + '">' +
      esc(c.name) + '<em>' + esc(c.code) + '</em></span>';
  }
  html += '</div>';
  box.innerHTML = html;
  $('chain-companies').classList.remove('hidden');
}

/* 厂商跳查 */
function chainSearch(code) {
  $('stock-input').value = code;
  hideSuggest();
  doSearch();
}
