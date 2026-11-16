import { bootApp } from './boot.js';

bootApp();

function convertPixivUrl(url) {
  const pathMatch = url.match(/\/[^/]+\/img\/(.+?)_p0/);
  if (!pathMatch) return url;
  return `https://i.pximg.net/img-original/img/${pathMatch[1]}_p0.png`;
}

async function fillFromArtworkId(id) {
  id = (id || '').trim();
  if (!id) return false;
  try {
    const cookie = (document.getElementById('cookie')?.value || '').trim();
    const headers = {};
    if (cookie) headers['X-Pixiv-Cookie'] = cookie;
    const proxyPort = (document.getElementById('proxyPort')?.value || '0').trim();
    if (proxyPort) headers['X-Proxy-Port'] = proxyPort;
    const res = await fetch(`/api/artwork-id/${encodeURIComponent(id)}`, { headers });
    const data = await res.json();
    if (!res.ok || data.success === false) {
      console.warn(data?.error || 'artwork-id 请求失败');
      return false;
    }
    const payload = data?.data ?? data;
    const body = payload?.body;
    if (!body) return false;
    if (body.userIllusts?.[id]?.url) {
      const u = convertPixivUrl(body.userIllusts[id].url);
      document.getElementById('url').value = u;
      localStorage.setItem('url', u);
    }
    if (body.pageCount) {
      document.getElementById('count').value = body.pageCount;
      localStorage.setItem('count', String(body.pageCount));
    }
    return true;
  } catch (e) {
    console.warn(e);
    return false;
  }
}

function resetProgressUI() {
  const progress = document.getElementById('progress');
  if (!progress) return;
  progress.innerHTML = `
    <div class="progress-stats">
      <div class="stat"><div class="stat-number">0</div><div class="stat-label">成功</div></div>
      <div class="stat"><div class="stat-number">0</div><div class="stat-label">失败</div></div>
      <div class="stat"><div class="stat-number">0/0</div><div class="stat-label">已完成</div></div>
      <div class="stat"><div class="stat-number">0%</div><div class="stat-label">进度</div></div>
    </div>
    <div class="progress-bar" role="progressbar" aria-label="下载进度" aria-valuemin="0" aria-valuemax="100" aria-valuenow="0">
      <svg class="progress-svg" viewBox="0 0 100 8" preserveAspectRatio="none" aria-hidden="true">
        <rect class="progress-fill" x="0" y="0" height="8" width="0"></rect>
      </svg>
    </div>`;
  hideDoneBanner();
}

function setDownloadControls(running, taskId, state) {
  const cancelBtn = document.getElementById('cancelDownload');
  const startBtn = document.getElementById('startDownload');
  const taskInput = document.getElementById('activeTaskId');
  const badge = document.getElementById('logBadge');
  if (taskInput) {
    if (taskId) taskInput.value = taskId;
    else if (!running) taskInput.value = '';
  }
  if (cancelBtn) cancelBtn.disabled = !running;
  if (startBtn) startBtn.disabled = !!running;
  if (badge) {
    if (running) {
      badge.dataset.state = 'running';
      badge.textContent = '进行中';
    } else if (state === 'complete') {
      badge.dataset.state = 'done';
      badge.textContent = '已完成';
    } else if (state === 'error' || state === 'cancelled') {
      badge.dataset.state = 'idle';
      badge.textContent = state === 'cancelled' ? '已中断' : '失败';
    } else {
      badge.dataset.state = 'idle';
      badge.textContent = '待命';
    }
  }
}

function activeTaskId() {
  return (document.getElementById('activeTaskId')?.value || '').trim();
}

function hideDoneBanner() {
  const bar = document.getElementById('downloadDoneBanner');
  if (bar) bar.hidden = true;
}

function showDoneBanner(fromNode) {
  const host = document.getElementById('logPanel');
  if (!host) return;
  let bar = document.getElementById('downloadDoneBanner');
  if (!bar) {
    bar = document.createElement('div');
    bar.id = 'downloadDoneBanner';
    bar.className = 'download-done-banner';
    bar.setAttribute('role', 'status');
    const header = host.querySelector('header.row');
    if (header && header.nextSibling) {
      host.insertBefore(bar, header.nextSibling);
    } else {
      host.prepend(bar);
    }
  }
  const msg = fromNode?.querySelector?.('.status-item-msg')?.textContent?.trim() || '任务已完成';
  const link = fromNode?.querySelector?.('.status-item-link');
  bar.innerHTML = '';
  const text = document.createElement('p');
  text.className = 'download-done-banner__text';
  text.textContent = msg;
  bar.appendChild(text);
  if (link) {
    const a = document.createElement('a');
    a.className = 'btn btn-primary download-done-banner__btn';
    a.href = link.getAttribute('href') || '#';
    a.textContent = link.textContent || '查看作品';
    bar.appendChild(a);
  }
  bar.hidden = false;
}

function onDownloadSubmitted() {
  hideDoneBanner();
  const els = document.querySelectorAll('#status [data-download-task]');
  const el = els[els.length - 1];
  const taskId = el?.getAttribute('data-download-task') || '';
  setDownloadControls(true, taskId);
  stickToBottom = true;
  scrollStatusToBottom(true);
}

window.fillFromArtworkId = fillFromArtworkId;
window.onDownloadSubmitted = onDownloadSubmitted;
window.resetProgressUI = resetProgressUI;

let stickToBottom = true;

function isStatusNearBottom(el, threshold = 80) {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= threshold;
}

function scrollStatusToBottom(force) {
  const el = document.getElementById('status');
  if (!el) return;
  if (!force && !stickToBottom) return;
  el.scrollTop = el.scrollHeight;
}

function applyStatusNode(node) {
  if (!(node instanceof HTMLElement)) return;
  const tid = node.getAttribute('data-download-task') || '';
  const state = node.getAttribute('data-download-state') || '';
  const active = activeTaskId();

  if (state === 'running' || (tid && !state)) {
    if (!active || !tid || tid === active) {
      setDownloadControls(true, tid || active);
    }
  }

  if (state === 'complete' || state === 'cancelled' || state === 'error') {
    if (active && tid && tid !== active) return;
    if (active && !tid && state === 'cancelled') return;
    setDownloadControls(false, '', state);
    if (state === 'complete') {
      stickToBottom = true;
      showDoneBanner(node);
      scrollStatusToBottom(true);
      try {
        if (document.hidden && typeof Notification !== 'undefined' && Notification.permission === 'granted') {
          new Notification('Pixiv Shelf', { body: '下载任务已完成' });
        }
      } catch {
        /* ignore */
      }
    }
  }
}

function applyLifecycleHint(json) {
  const state = json?.downloadState;
  if (!state) return;
  const tid = typeof json.taskId === 'string' ? json.taskId : '';
  const active = activeTaskId();
  if (active && tid && tid !== active) return;
  setDownloadControls(false, '', state);
  if (state !== 'complete') return;
  stickToBottom = true;
  const lastComplete = [...document.querySelectorAll('#status [data-download-state="complete"]')].pop();
  if (lastComplete) {
    showDoneBanner(lastComplete);
  } else {
    const fake = document.createElement('div');
    const p = document.createElement('p');
    p.className = 'status-item-msg';
    const ok = typeof json.success === 'number' ? json.success : 0;
    const fail = typeof json.fail === 'number' ? json.fail : 0;
    p.textContent = `任务完成：成功 ${ok}，失败 ${fail}。`;
    fake.appendChild(p);
    showDoneBanner(fake);
  }
  scrollStatusToBottom(true);
}

function observeStatusLog() {
  const el = document.getElementById('status');
  if (!el || el.__statusObserved) return;
  el.__statusObserved = true;
  el.addEventListener(
    'scroll',
    () => {
      stickToBottom = isStatusNearBottom(el);
    },
    { passive: true }
  );
  const mo = new MutationObserver((mutations) => {
    let forceFollow = false;
    for (const m of mutations) {
      for (const node of m.addedNodes) {
        if (!(node instanceof HTMLElement)) continue;
        applyStatusNode(node);
        if (
          node.getAttribute('data-download-state') === 'complete' ||
          node.getAttribute('data-download-state') === 'cancelled' ||
          node.getAttribute('data-download-state') === 'error' ||
          node.textContent?.includes('任务完成')
        ) {
          forceFollow = true;
        }
        for (const child of node.querySelectorAll?.('[data-download-state]') || []) {
          applyStatusNode(child);
        }
      }
    }
    scrollStatusToBottom(forceFollow);
  });
  mo.observe(el, { childList: true, subtree: true });
}

function appendStatusHint(message) {
  const el = document.getElementById('status');
  if (!el) return;
  const placeholder = el.querySelector('.status-placeholder');
  if (placeholder) placeholder.remove();
  const item = document.createElement('div');
  item.className = 'status-item status-start';
  item.innerHTML = `<span class="status-item-icon" aria-hidden="true">·</span><div class="status-item-body"><p class="status-item-msg"></p></div>`;
  item.querySelector('.status-item-msg').textContent = message;
  el.appendChild(item);
  stickToBottom = true;
  scrollStatusToBottom(true);
}

function isHxWsOpen() {
  if (liveWS && liveWS.readyState === WebSocket.OPEN) return true;
  return false;
}

function connectShell() {
  return document.querySelector('[hx-ws\\:connect], [hx-ws-connect]');
}

/** Track the live hx-ws connection object from official events. */
let liveWS = null;
let liveConnection = null;
let lastWsEventAt = 0;
let reconnectForceTimer = 0;

function markWsAlive() {
  lastWsEventAt = Date.now();
}

function socketFromConnection(connection) {
  return connection?.socket || null;
}

/**
 * Force hx-ws to reconnect without modifying the extension:
 * close the live socket (triggers official reconnect), or re-process the connect element.
 */
function forceHxWsReconnect(reason) {
  const shell = connectShell();
  if (!shell) return;

  const sock = liveWS || socketFromConnection(liveConnection);
  if (sock && (sock.readyState === WebSocket.OPEN || sock.readyState === WebSocket.CONNECTING)) {
    try {
      sock.close(4000, reason || 'watchdog');
    } catch {
      /* ignore */
    }
    return;
  }

  // Connection object gone — re-init via official htmx.process + load trigger.
  if (shell._htmx?.ws) {
    shell._htmx.ws.url = null;
    shell._htmx.ws.initialized = false;
  }
  if (typeof window.htmx?.process === 'function') {
    window.htmx.process(shell);
  }
  // Explicitly fire load-style connect if process alone does not.
  try {
    shell.dispatchEvent(new Event('load', { bubbles: false }));
  } catch {
    /* ignore */
  }
}

function ensureWsHealthy(reason) {
  if (isHxWsOpen()) {
    markWsAlive();
    return;
  }
  forceHxWsReconnect(reason || 'ensure');
}

function waitForWs(timeoutMs = 8000) {
  return new Promise((resolve) => {
    if (isHxWsOpen()) {
      resolve(true);
      return;
    }
    ensureWsHealthy('wait');
    const onOpen = () => {
      cleanup();
      resolve(true);
    };
    const timer = window.setTimeout(() => {
      cleanup();
      resolve(isHxWsOpen());
    }, timeoutMs);
    function cleanup() {
      window.clearTimeout(timer);
      document.removeEventListener('htmx:after:ws:connection', onOpen);
    }
    document.addEventListener('htmx:after:ws:connection', onOpen);
  });
}

document.addEventListener('htmx:after:ws:connection', (event) => {
  liveConnection = event.detail?.connection || null;
  liveWS = socketFromConnection(liveConnection);
  markWsAlive();
});

document.addEventListener('htmx:ws:close', () => {
  // hx-ws schedules reconnect itself; keep references until open again.
  if (liveWS && liveWS.readyState === WebSocket.CLOSED) {
    liveWS = null;
  }
});

document.addEventListener('htmx:after:ws:message', (event) => {
  markWsAlive();
  const json = event.detail?.message?.json;
  if (json && typeof json === 'object') {
    applyLifecycleHint(json);
  }
  scrollStatusToBottom(false);
});

document.addEventListener('visibilitychange', () => {
  if (document.hidden) return;
  // Tab visible again: recover from half-open / missed reconnect.
  window.setTimeout(() => ensureWsHealthy('visible'), 50);
});

window.addEventListener('online', () => {
  ensureWsHealthy('online');
});

window.addEventListener('pageshow', () => {
  try {
    ensureWsHealthy('pageshow');
  } catch {
    /* ignore */
  }
});

// Periodic check while a download is marked running (catches silent half-open sockets).
window.setInterval(() => {
  const running = !!document.getElementById('cancelDownload') && !document.getElementById('cancelDownload').disabled;
  if (!running) return;
  if (isHxWsOpen()) return;
  if (reconnectForceTimer) return;
  reconnectForceTimer = window.setTimeout(() => {
    reconnectForceTimer = 0;
  }, 2000);
  forceHxWsReconnect('download-running');
}, 5000);

async function restoreForm() {
  for (const key of ['url', 'count', 'taskName', 'cookie', 'proxyPort']) {
    const v = localStorage.getItem(key);
    if (v) {
      const input = document.getElementById(key);
      if (input) input.value = v;
    }
  }

  let wantRedownload = false;
  let taskName = '';
  try {
    const params = new URLSearchParams(location.search);
    taskName = (params.get('taskName') || '').trim();
    wantRedownload = params.get('redownload') === '1';
    if (taskName) {
      const prevTask = (localStorage.getItem('taskName') || '').trim();
      if (prevTask !== taskName) {
        const urlEl = document.getElementById('url');
        const countEl = document.getElementById('count');
        if (urlEl) urlEl.value = '';
        if (countEl) countEl.value = '';
      }
      document.getElementById('taskName').value = taskName;
      localStorage.setItem('taskName', taskName);
    }
    if (wantRedownload) {
      const box = document.getElementById('redownload');
      if (box) box.checked = true;
    }
  } catch {
    /* ignore */
  }

  observeStatusLog();

  if (taskName) {
    appendStatusHint(wantRedownload ? '正在根据作品 ID 准备重新下载…' : '正在根据作品 ID 填充下载信息…');
    await fillFromArtworkId(taskName);
  }

  if (wantRedownload && taskName) {
    const url = (document.getElementById('url')?.value || '').trim();
    const cookie = (document.getElementById('cookie')?.value || '').trim();
    if (!url) {
      appendStatusHint('未能自动填充原图 URL，请手动填写后点击「开始下载」。');
      return;
    }
    if (!cookie) {
      appendStatusHint('请先填写 Pixiv Cookie，再点击「开始下载」。');
      return;
    }
    await waitForWs();
    const form = document.getElementById('downloadForm');
    if (form && typeof form.requestSubmit === 'function') {
      appendStatusHint('信息已就绪，开始重新下载…');
      form.requestSubmit();
    }
  }
}

restoreForm();
