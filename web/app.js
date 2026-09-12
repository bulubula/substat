let cachedData = [];
const SLOT_COUNT = 60;

// Theme switcher logic
function initTheme() {
  const saved = localStorage.getItem('substat-theme') || 'auto';
  applyTheme(saved);

  document.querySelectorAll('.theme-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const mode = btn.dataset.themeVal;
      applyTheme(mode);
    });
  });

  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (localStorage.getItem('substat-theme') === 'auto' || !localStorage.getItem('substat-theme')) {
      applyTheme('auto');
    }
  });
}

function applyTheme(mode) {
  localStorage.setItem('substat-theme', mode);
  document.querySelectorAll('.theme-btn').forEach(b => {
    b.classList.toggle('active', b.dataset.themeVal === mode);
  });

  let effectiveTheme = mode;
  if (mode === 'auto') {
    effectiveTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  document.documentElement.setAttribute('data-theme', effectiveTheme);
}

// Fetch & render
async function fetchStatus() {
  try {
    const res = await fetch('api/status');
    if (!res.ok) throw new Error('API request failed');
    cachedData = await res.json();
    renderMonitors(cachedData);
  } catch (err) {
    console.error(err);
    document.getElementById('monitors').innerHTML = `<div class="card" style="color:var(--down)">数据获取失败: ${err.message}</div>`;
  }
}

function parseIntervalToMs(intervalStr) {
  if (!intervalStr) return 60000;
  const unit = intervalStr.slice(-1);
  const val = parseInt(intervalStr.slice(0, -1), 10);
  if (isNaN(val)) return 60000;
  if (unit === 'm') return val * 60 * 1000;
  if (unit === 'h') return val * 3600 * 1000;
  if (unit === 'd') return val * 86400 * 1000;
  return 60000;
}

// Build strictly 60 slots: right-aligned with latest points, left-padded with empty
function buildTimelineSlots(points) {
  const slots = new Array(SLOT_COUNT);
  const pts = points || [];
  const startEmptyCount = Math.max(0, SLOT_COUNT - pts.length);

  // Left-pad with empty slots
  for (let i = 0; i < startEmptyCount; i++) {
    slots[i] = {
      slotIndex: i,
      point: null
    };
  }

  // Right side filled with real points (oldest to newest)
  // If points exceed 60, take the last 60
  const recentPoints = pts.slice(-SLOT_COUNT);
  for (let i = 0; i < recentPoints.length; i++) {
    slots[startEmptyCount + i] = {
      slotIndex: startEmptyCount + i,
      point: recentPoints[i]
    };
  }

  return slots;
}

function renderMonitors(monitors) {
  const container = document.getElementById('monitors');
  if (!monitors || monitors.length === 0) {
    container.innerHTML = '<div class="loading">暂无配置的监控服务</div>';
    return;
  }

  container.innerHTML = monitors.map((m, mIndex) => {
    const points = m.history || [];
    const degradedThreshold = m.degraded_latency_ms || 1000;
    const slots = buildTimelineSlots(points);

    // Save slots on monitor object for click handler
    m._slots = slots;

    const bars = slots.map((s, sIndex) => {
      const p = s.point;
      if (!p) {
        return `<div class="bar-point empty" title="等待更多采样数据 (未满60次)"></div>`;
      }

      let cls = 'fail';
      if (p.success) {
        cls = (p.latency_ms > degradedThreshold) ? 'degraded' : 'success';
      }
      const timeStr = new Date(p.time).toLocaleTimeString();
      const title = `${timeStr} | ${p.success ? '成功' : '失败'} | 延迟: ${p.latency_ms}ms${p.message ? ' (' + p.message + ')' : ''}`;
      return `<div class="bar-point ${cls}" title="${title}" onclick="showSlotDetail(${mIndex}, ${sIndex})"></div>`;
    }).join('');

    const lastTime = m.last_check ? new Date(m.last_check).toLocaleString() : '等待首次检测';
    const statusText = m.status === 'UP' ? '正常 (UP)' : (m.status === 'DEGRADED' ? '延迟过高 (DEGRADED)' : m.status);

    return `
      <div class="card" id="card-${mIndex}">
        <div class="card-header">
          <div class="card-title">
            <span>${escapeHTML(m.name)}</span>
            <span class="probe-type">${escapeHTML(m.type)}</span>
          </div>
          <span class="badge ${m.status}">${statusText}</span>
        </div>
        <div class="card-body">
          <span>最新监测: <strong>${m.last_latency_ms} ms</strong></span>
          <span>平均延迟: <strong>${m.avg_latency_ms || 0} ms</strong></span>
          <span>监测间隔: <strong>${escapeHTML(m.interval || '-')}</strong></span>
          <span>最近检测: ${lastTime}</span>
          ${m.last_message ? `<div style="width:100%;color:var(--down)">异常信息: ${escapeHTML(m.last_message)}</div>` : ''}
        </div>
        <div class="history-bar">
          ${bars}
        </div>
        <div id="point-detail-${mIndex}" class="point-detail-box"></div>
      </div>
    `;
  }).join('');
}

function showSlotDetail(mIndex, sIndex) {
  const m = cachedData[mIndex];
  if (!m || !m._slots || !m._slots[sIndex]) return;

  const slot = m._slots[sIndex];
  const p = slot.point;
  const box = document.getElementById(`point-detail-${mIndex}`);
  if (!box) return;

  if (!p) {
    const timeStr = new Date(slot.expectedTime).toLocaleString();
    box.innerHTML = `
      <div style="display:flex;justify-content:space-between;align-items:center;">
        <span><strong>时间槽:</strong> ${timeStr}</span>
        <span style="color:var(--text-muted);font-weight:bold;">● 无采样记录 / 服务未启动</span>
      </div>
    `;
    box.classList.add('show');
    return;
  }

  const timeStr = new Date(p.time).toLocaleString();
  const degradedThreshold = m.degraded_latency_ms || 1000;
  let statusBadge = '<span style="color:#2ea043;font-weight:bold;">● 正常 (<= ' + degradedThreshold + 'ms)</span>';
  if (!p.success) {
    statusBadge = '<span style="color:#f85149;font-weight:bold;">● 不可达 / 失败</span>';
  } else if (p.latency_ms > degradedThreshold) {
    statusBadge = '<span style="color:#d29922;font-weight:bold;">● 延迟过高 (> ' + degradedThreshold + 'ms)</span>';
  }

  box.innerHTML = `
    <div style="display:flex;justify-content:space-between;align-items:center;">
      <span><strong>采样时间:</strong> ${timeStr}</span>
      <span>${statusBadge}</span>
    </div>
    <div style="margin-top:0.3rem;display:flex;gap:1.5rem;color:var(--text-muted);">
      <span>延迟: <strong style="color:var(--text);">${p.latency_ms} ms</strong></span>
      ${p.message ? `<span>详情: ${escapeHTML(p.message)}</span>` : ''}
    </div>
  `;
  box.classList.add('show');
}

function escapeHTML(str) {
  if (!str) return '';
  return str.replace(/[&<>'"]/g, 
    tag => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[tag] || tag)
  );
}

// Bootstrap
initTheme();
fetchStatus();
setInterval(fetchStatus, 10000);
