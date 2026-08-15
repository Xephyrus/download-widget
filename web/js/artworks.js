import { bootApp } from './boot.js';

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

function loadGallery(attempt = 0) {
  if (!artworkId) {
    statusEl.textContent = '未识别 artwork ID。';
    galleryEl.innerHTML = '';
    return;
  }
  if (hasSSRGallery()) {
    onGalleryReady();
    return;
  }
  if (!window.htmx) {
    if (attempt < 50) {
      setTimeout(() => loadGallery(attempt + 1), 40);
      return;
    }
    statusEl.textContent = 'HTMX 未加载，请刷新页面。';
    return;
  }
  if (!statusEl.textContent.trim()) {
    statusEl.textContent = '加载中…';
  }
  const req = window.htmx.ajax('GET', `/partials/artworks/${encodeURIComponent(artworkId)}`, {
    target: '#gallery',
    swap: 'innerHTML',
  });
  if (req && typeof req.then === 'function') {
    req
      .then(() => {
        galleryEl.setAttribute('data-ssr', '1');
        onGalleryReady();
      })
      .catch(() => {
        statusEl.textContent = '加载失败，请稍后重试。';
      });
  } else {
    setTimeout(onGalleryReady, 0);
  }
}

function boot() {
  document.body.addEventListener('htmx:afterSwap', (e) => {
    if (e.detail?.target?.id === 'gallery') {
      galleryEl.setAttribute('data-ssr', '1');
      onGalleryReady();
    }
  });
  document.body.addEventListener('htmx:responseError', () => {
    if (!hasSSRGallery()) {
      statusEl.textContent = '加载失败，请稍后重试。';
    }
  });

  if (hasSSRGallery()) {
    onGalleryReady();
    return;
  }
  loadGallery();
}

boot();

window.addEventListener('pageshow', (e) => {
  if (e.persisted) {
    if (window.Fancybox) window.Fancybox.close(true);
    onGalleryReady();
  }
});
