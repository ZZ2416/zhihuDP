/* video.js —— 相关视频（B站）：加载 + 按「最新 / 播放量」排序切换 */
let videoItems = [];
let videoSort = 'time'; // time | play

/* 加载相关视频（一次拉取，前端排序切换） */
async function loadVideo(name) {
  if (!name) return;
  const card = $('video-card');
  if (!card) return;
  card.classList.remove('hidden');
  $('video-body').innerHTML = '<span class="fin-loading">加载相关视频…</span>';
  try {
    // apiVideo 直接返回解析后的 JSON（null=接口失败）
    const items = await apiVideo(name, 15);
    if (!items || !items.length) {
      $('video-body').innerHTML = '<span class="fin-loading">暂无相关视频</span>';
      return;
    }
    videoItems = items;
    renderVideos();
  } catch (e) {
    $('video-body').innerHTML =
      '<div class="degraded" style="margin-top:4px">视频加载失败：' + esc(e.message) + '</div>';
  }
}

/* 排序切换 */
function sortVideos(mode) {
  videoSort = mode;
  const t = $('vs-time'), p = $('vs-play');
  if (mode === 'play') { t.classList.remove('active'); p.classList.add('active'); }
  else { p.classList.remove('active'); t.classList.add('active'); }
  renderVideos();
}

/* 渲染：时间（新→旧）或播放量（大→小） */
function renderVideos() {
  const items = [...videoItems];
  if (videoSort === 'time') {
    items.sort((a, b) => (b.publish_time || '').localeCompare(a.publish_time || ''));
  } else {
    items.sort((a, b) => (b.play || 0) - (a.play || 0));
  }
  let html = '<div class="video-list">';
  for (const v of items) {
    html += '<div class="video-item">' +
      '<a class="t" href="' + esc(v.url) + '" target="_blank" rel="noopener" title="打开视频">' + esc(v.title) + '</a>' +
      '<span class="meta">▶ ' + fmtPlay(v.play) + ' · ' + esc(v.duration) + ' · ' + esc(v.author || '') + '</span>' +
      '<span class="date">' + esc((v.publish_time || '').slice(0, 16)) + '</span>' +
      '</div>';
  }
  html += '</div>';
  $('video-body').innerHTML = html;
}

/* 播放量格式化：1.2万 / 3.4亿 */
function fmtPlay(n) {
  n = Number(n) || 0;
  if (n >= 100000000) return (n / 100000000).toFixed(1) + '亿';
  if (n >= 10000) return (n / 10000).toFixed(1) + '万';
  return String(n);
}
