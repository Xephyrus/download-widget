import { bootApp } from './boot.js';

bootApp();

function convertPixivUrl(url) {
  const pathMatch = url.match(/\/[^/]+\/img\/(.+?)_p0/);
  if (!pathMatch) return url;
  return `https://i.pximg.net/img-original/img/${pathMatch[1]}_p0.png`;
}

async function fillFromArtworkId(id) {
  id = (id || '').trim();
  if (!id) return;
  try {
    const cookie = (document.getElementById('cookie')?.value || '').trim();
    const headers = {};
    if (cookie) headers['X-Pixiv-Cookie'] = cookie;
    const res = await fetch(`/api/artwork-id/${encodeURIComponent(id)}`, { headers });
    const data = await res.json();
    if (!res.ok || data.success === false) {
      console.warn(data?.error || 'artwork-id 请求失败');
      return;
    }
    const payload = data?.data ?? data;
    const body = payload?.body;
    if (!body) return;
    if (body.userIllusts?.[id]?.url) {
      const u = convertPixivUrl(body.userIllusts[id].url);
      document.getElementById('url').value = u;
      localStorage.setItem('url', u);
    }
    if (body.pageCount) {
      document.getElementById('count').value = body.pageCount;
      localStorage.setItem('count', String(body.pageCount));
    }
  } catch (e) {
    console.warn(e);
  }
}

function setDownloadControls(running, taskId) {
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
    badge.dataset.state = running ? 'running' : 'idle';
    badge.textContent = running ? '进行中' : '待命';
  }
}

function onDownloadSubmitted() {
  const el = document.querySelector('#status [data-download-task]:last-of-type');
  const taskId = el?.getAttribute('data-download-task');
  if (taskId) setDownloadControls(true, taskId);
}

// hx-on attributes still call these from HTML.
window.fillFromArtworkId = fillFromArtworkId;
window.onDownloadSubmitted = onDownloadSubmitted;

function watchDownloadLifecycle() {
  const el = document.getElementById('status');
  if (!el || el.__lifecycleWatch) return;
  el.__lifecycleWatch = true;
  const observer = new MutationObserver((mutations) => {
    for (const m of mutations) {
      for (const node of m.addedNodes) {
        if (!(node instanceof HTMLElement)) continue;
        const taskEl = node.matches?.('[data-download-task]')
          ? node
          : node.querySelector?.('[data-download-task]');
        if (taskEl) {
          setDownloadControls(true, taskEl.getAttribute('data-download-task'));
        }
        const state =
          node.getAttribute?.('data-download-state') ||
          node.querySelector?.('[data-download-state]')?.getAttribute('data-download-state');
        if (state === 'complete' || state === 'cancelled' || state === 'error') {
          setDownloadControls(false, '');
        }
        const text = node.textContent || '';
        if (
          text.includes('任务完成') ||
          text.includes('下载已中断') ||
          text.includes('任务错误')
        ) {
          setDownloadControls(false, '');
        }
      }
    }
  });
  observer.observe(el, { childList: true, subtree: true });
}

function isStatusNearBottom(el, threshold = 80) {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= threshold;
}

function scrollStatusToBottom() {
  const el = document.getElementById('status');
  if (!el) return;
  el.scrollTop = el.scrollHeight;
}

function pinStatusLogScroll() {
  const el = document.getElementById('status');
  if (!el || el.__scrollPinned) return;
  el.__scrollPinned = true;

  let stickToBottom = true;
  el.addEventListener(
    'scroll',
    () => {
      stickToBottom = isStatusNearBottom(el);
    },
    { passive: true }
  );

  const observer = new MutationObserver(() => {
    if (!stickToBottom) return;
    requestAnimationFrame(scrollStatusToBottom);
  });
  observer.observe(el, { childList: true, subtree: true });

  function followAfterSwap(e) {
    if (!(e.detail?.target?.id === 'status')) return;
    stickToBottom = true;
    requestAnimationFrame(scrollStatusToBottom);
  }
  document.body.addEventListener('htmx:afterSwap', followAfterSwap);
}

function restoreForm() {
  for (const key of ['url', 'count', 'taskName', 'cookie']) {
    const v = localStorage.getItem(key);
    if (v) {
      const input = document.getElementById(key);
      if (input) input.value = v;
    }
  }
  try {
    const params = new URLSearchParams(location.search);
    const taskName = (params.get('taskName') || '').trim();
    if (taskName) {
      document.getElementById('taskName').value = taskName;
      localStorage.setItem('taskName', taskName);
      fillFromArtworkId(taskName);
    }
    if (params.get('redownload') === '1') {
      const box = document.getElementById('redownload');
      if (box) box.checked = true;
    }
  } catch {
    /* ignore */
  }
  pinStatusLogScroll();
  watchDownloadLifecycle();
}

restoreForm();
