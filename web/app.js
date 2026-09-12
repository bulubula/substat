async function fetchStatus() {
  try {
    const res = await fetch('api/status');
    if (!res.ok) throw new Error('API request failed');
    const data = await res.json();
    renderMonitors(data);
  } catch (err) {
    console.error(err);
    document.getElementById('monitors').innerHTML = `<div class="card" style="color:var(--down)">数据获取失败: ${err.message}</div>`;
  }
}

function renderMonitors(monitors) {
  const container = document.getElementById('monitors');
  if (!monitors || monitors.length === 0) {
    container.innerHTML = '<div class="loading">暂无配置的监控服务</div>';
    return;
  }

  container.innerHTML = monitors.map(m => {
    const points = m.history || [];
    const bars = points.map(p => {
      const cls = p.success ? 'success' : 'fail';
      const timeStr = new Date(p.time).toLocaleTimeString();
      const title = `${timeStr}: ${p.latency_ms}ms ${p.message || ''}`;
      return `<div class="bar-point ${cls}" title="${title}"></div>`;
    }).join('');

    const lastTime = m.last_check ? new Date(m.last_check).toLocaleString() : '等待首次检测';

    return `
      <div class="card">
        <div class="card-header">
          <div class="card-title">
            <span>${escapeHTML(m.name)}</span>
            <span class="probe-type">${escapeHTML(m.type)}</span>
          </div>
          <span class="badge ${m.status}">${m.status}</span>
        </div>
        <div class="card-body">
          <span>延迟: <strong>${m.last_latency_ms} ms</strong></span>
          <span>最新检测: ${lastTime}</span>
          ${m.last_message ? `<span style="color:var(--down)">${escapeHTML(m.last_message)}</span>` : ''}
        </div>
        <div class="history-bar">
          ${bars || '<div style="font-size:0.7rem;color:var(--text-muted);padding-left:4px">积累历史采样中...</div>'}
        </div>
      </div>
    `;
  }).join('');
}

function escapeHTML(str) {
  if (!str) return '';
  return str.replace(/[&<>'"]/g, 
    tag => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[tag] || tag)
  );
}

// Initial fetch and 10s auto-refresh
fetchStatus();
setInterval(fetchStatus, 10000);
