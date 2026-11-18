import { bootApp } from './boot.js?v=36';
import { mountAllTagsEditors } from './tags-editor.js?v=36';

bootApp();

const artworkId = decodeURIComponent(location.pathname.replace(/^\/artworks\//, '')).replace(
  /\/$/,
  ''
);
const statusEl = document.getElementById('status');
const galleryEl = document.getElementById('gallery');
if (artworkId) {
  const idEl = document.getElementById('artworkId');
  if (idEl) idEl.textContent = artworkId;
}

const fancyboxOpts = {
  Hash: true,
  Carousel: {
    preload: 2,
    Toolbar: {
      display: {
        left: ['counter'],
        middle: [],
        right: [
          'zoomIn',
          'zoomOut',
          'toggle1to1',
          'rotateCCW',
          'rotateCW',
          'autoplay',
          'fullscreen',
          'thumbs',
          'download',
          'close',
        ],
      },
    },
    Thumbs: {
      type: 'classic',
    },
    breakpoints: {
      '(max-width: 719px)': {
        Arrows: false,
        Toolbar: {
          display: {
            left: ['counter'],
            middle: [],
            right: ['thumbs', 'download', 'close'],
          },
        },
      },
    },
  },
};

function bindFancybox(attempt = 0) {
  if (!window.Fancybox) {
    if (attempt < 50) {
      setTimeout(() => bindFancybox(attempt + 1), 40);
    }
    return;
  }
  window.Fancybox.unbind('[data-fancybox="gallery"]');
  window.Fancybox.bind('[data-fancybox="gallery"]', fancyboxOpts);
}

function onGalleryReady() {
  bindFancybox();
  window.dispatchEvent(new Event('resize'));
}

function hasSSRGallery() {
  if (galleryEl.getAttribute('data-ssr') === '1') return true;
  return !!(
    galleryEl.querySelector('a[data-fancybox="gallery"]') ||
    galleryEl.querySelector('.gallery-empty')
  );
}

function loadGallery() {
  if (!artworkId || !window.htmx) {
    if (!window.htmx) {
      setTimeout(loadGallery, 40);
    }
    return;
  }
  statusEl.textContent = '加载中…';
  const req = window.htmx.ajax('GET', `/partials/artworks/${encodeURIComponent(artworkId)}`, {
    target: '#gallery',
    swap: 'innerHTML',
  });
  if (req && typeof req.then === 'function') {
    req.then(onGalleryReady).catch(() => {
      statusEl.textContent = '加载失败，请稍后重试。';
    });
  } else {
    setTimeout(onGalleryReady, 0);
  }
}

function tagsPanelReady() {
  const panel = document.getElementById('tagsPanel');
  return !!(panel && panel.hasAttribute('data-tags-editor') && panel.querySelector('[data-tags-input]'));
}

function ensureTagsPanel(attempt = 0) {
  if (!artworkId) return;
  if (tagsPanelReady()) {
    mountAllTagsEditors();
    return;
  }
  if (!window.htmx) {
    if (attempt < 50) setTimeout(() => ensureTagsPanel(attempt + 1), 40);
    return;
  }
  const req = window.htmx.ajax('GET', `/partials/folders/${encodeURIComponent(artworkId)}/tags`, {
    target: '#tagsPanel',
    swap: 'outerHTML',
  });
  const done = () => mountAllTagsEditors();
  if (req && typeof req.then === 'function') {
    req.then(done).catch(() => {
      const panel = document.getElementById('tagsPanel');
      const empty = panel?.querySelector('.tags-empty, .panel-desc');
      if (empty) empty.textContent = '标签加载失败，请刷新重试。';
    });
  } else {
    setTimeout(done, 0);
  }
}

function boot() {
  document.body.addEventListener('htmx:after:swap', (e) => {
    const target = e.detail?.target;
    if (target?.id === 'gallery') {
      galleryEl.setAttribute('data-ssr', '1');
      onGalleryReady();
    }
    if (
      target?.id === 'tagsPanel' ||
      target?.closest?.('#tagsPanel') ||
      target?.querySelector?.('[data-tags-editor]')
    ) {
      mountAllTagsEditors(document);
    }
  });
  document.body.addEventListener('htmx:response:error', () => {
    if (!hasSSRGallery()) {
      statusEl.textContent = '加载失败，请稍后重试。';
    }
  });

  mountAllTagsEditors();
  ensureTagsPanel();

  if (hasSSRGallery()) {
    onGalleryReady();
    return;
  }
  loadGallery();
}

boot();

function closeFancyboxSafe() {
  try {
    if (window.Fancybox) window.Fancybox.close(true);
  } catch {
    /* ignore teardown races on Back / bfcache */
  }
}

window.addEventListener('pagehide', () => {
  closeFancyboxSafe();
});

window.addEventListener('pageshow', (e) => {
  if (e.persisted) {
    closeFancyboxSafe();
    onGalleryReady();
  }
});
