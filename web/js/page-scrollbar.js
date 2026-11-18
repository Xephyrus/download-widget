/**
 * Overlay scrollbar — window ([data-page-scrollbar]) and element
 * ([data-scrollbar-scroller] + [data-custom-scrollbar]).
 */

function createOverlayScrollbar(root, scroller) {
  const bar = root.querySelector('[data-scrollbar-bar]');
  const thumb = root.querySelector('[data-scrollbar-thumb]');
  if (!bar || !thumb) {
    return { destroy() {} };
  }

  const isWindow = !scroller || scroller === window;
  let dragging = false;
  let dragStartY = 0;
  let dragStartScroll = 0;
  let raf = 0;

  const docEl = () => document.documentElement;

  function getScrollTop() {
    if (isWindow) return window.scrollY || docEl().scrollTop;
    return scroller.scrollTop;
  }

  function getView() {
    return isWindow ? window.innerHeight : scroller.clientHeight;
  }

  function getTotal() {
    if (!isWindow) return scroller.scrollHeight;
    const el = docEl();
    return Math.max(
      el.scrollHeight,
      el.offsetHeight,
      document.body ? document.body.scrollHeight : 0,
      document.body ? document.body.offsetHeight : 0
    );
  }

  function setScrollTop(top) {
    if (isWindow) {
      window.scrollTo({ top, behavior: 'auto' });
    } else {
      scroller.scrollTop = top;
    }
  }

  function trackHeight() {
    const h = bar.clientHeight;
    if (h > 0) return h;
    return Math.max(0, getView() - 12);
  }

  function metrics() {
    const scrollTop = getScrollTop();
    const view = getView();
    const total = getTotal();
    const maxScroll = Math.max(0, total - view);
    const barH = trackHeight();
    const ratio = total > 0 ? view / total : 1;
    const thumbH = Math.max(28, Math.round(barH * Math.min(1, ratio)));
    const track = Math.max(0, barH - thumbH);
    const thumbTop = maxScroll > 0 ? (scrollTop / maxScroll) * track : 0;
    return { scrollTop, view, total, maxScroll, thumbH, thumbTop, track };
  }

  function render() {
    if (isWindow && docEl().classList.contains('with-fancybox')) {
      root.hidden = true;
      root.classList.remove('is-active');
      return;
    }

    const view = getView();
    const total = getTotal();
    const maxScroll = Math.max(0, total - view);
    const needed = maxScroll > 1;

    if (!needed) {
      root.hidden = true;
      root.classList.remove('is-active');
      return;
    }

    root.hidden = false;
    void root.offsetHeight;

    const m = metrics();
    thumb.style.height = `${m.thumbH}px`;
    thumb.style.transform = `translate(-50%, ${m.thumbTop}px)`;
  }

  function onScrollOrResize() {
    if (raf) cancelAnimationFrame(raf);
    raf = requestAnimationFrame(() => {
      raf = 0;
      render();
      requestAnimationFrame(render);
    });
  }

  function onThumbPointerDown(e) {
    e.preventDefault();
    e.stopPropagation();
    dragging = true;
    root.classList.add('is-active');
    dragStartY = e.clientY;
    dragStartScroll = getScrollTop();
    thumb.setPointerCapture(e.pointerId);
  }

  function onThumbPointerMove(e) {
    if (!dragging) return;
    const m = metrics();
    if (m.track <= 0 || m.maxScroll <= 0) return;
    const delta = e.clientY - dragStartY;
    const next = dragStartScroll + (delta / m.track) * m.maxScroll;
    setScrollTop(Math.min(m.maxScroll, Math.max(0, next)));
  }

  function onThumbPointerUp(e) {
    if (!dragging) return;
    dragging = false;
    root.classList.remove('is-active');
    try {
      thumb.releasePointerCapture(e.pointerId);
    } catch {
      /* ignore */
    }
  }

  function onBarPointerDown(e) {
    if (e.target === thumb || thumb.contains(e.target)) return;
    e.preventDefault();
    e.stopPropagation();
    const m = metrics();
    if (m.track <= 0 || m.maxScroll <= 0) return;
    const rect = bar.getBoundingClientRect();
    const y = e.clientY - rect.top - m.thumbH / 2;
    const ratio = Math.min(1, Math.max(0, y / m.track));
    setScrollTop(ratio * m.maxScroll);
  }

  const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(onScrollOrResize) : null;
  if (ro) {
    if (isWindow) {
      ro.observe(document.body);
      ro.observe(docEl());
    } else {
      ro.observe(scroller);
      if (scroller.firstElementChild) ro.observe(scroller.firstElementChild);
    }
  }

  if (isWindow) {
    window.addEventListener('scroll', onScrollOrResize, { passive: true });
  } else {
    scroller.addEventListener('scroll', onScrollOrResize, { passive: true });
  }
  window.addEventListener('resize', onScrollOrResize);
  window.addEventListener('load', onScrollOrResize);
  thumb.addEventListener('pointerdown', onThumbPointerDown);
  thumb.addEventListener('pointermove', onThumbPointerMove);
  thumb.addEventListener('pointerup', onThumbPointerUp);
  thumb.addEventListener('pointercancel', onThumbPointerUp);
  bar.addEventListener('pointerdown', onBarPointerDown);
  document.body.addEventListener('htmx:after:swap', onScrollOrResize);
  document.body.addEventListener('htmx:after:settle', onScrollOrResize);

  const mo =
    typeof MutationObserver !== 'undefined' ? new MutationObserver(onScrollOrResize) : null;
  if (mo && isWindow) {
    mo.observe(docEl(), { attributes: true, attributeFilter: ['class'] });
  }

  onScrollOrResize();

  return {
    destroy() {
      if (raf) cancelAnimationFrame(raf);
      if (ro) ro.disconnect();
      if (mo) mo.disconnect();
      if (isWindow) {
        window.removeEventListener('scroll', onScrollOrResize);
      } else {
        scroller.removeEventListener('scroll', onScrollOrResize);
      }
      window.removeEventListener('resize', onScrollOrResize);
      window.removeEventListener('load', onScrollOrResize);
      thumb.removeEventListener('pointerdown', onThumbPointerDown);
      thumb.removeEventListener('pointermove', onThumbPointerMove);
      thumb.removeEventListener('pointerup', onThumbPointerUp);
      thumb.removeEventListener('pointercancel', onThumbPointerUp);
      bar.removeEventListener('pointerdown', onBarPointerDown);
      document.body.removeEventListener('htmx:after:swap', onScrollOrResize);
      document.body.removeEventListener('htmx:after:settle', onScrollOrResize);
    },
  };
}

function preferNativeScrollbar() {
  try {
    return (
      window.matchMedia('(max-width: 720px)').matches ||
      window.matchMedia('(hover: none) and (pointer: coarse)').matches
    );
  } catch {
    return false;
  }
}

let apis = [];

function destroyAll() {
  for (const api of apis) {
    if (api && typeof api.destroy === 'function') api.destroy();
  }
  apis = [];
}

export function mountPageScrollbars() {
  if (preferNativeScrollbar()) {
    destroyAll();
    return;
  }
  if (apis.length) {
    window.dispatchEvent(new Event('resize'));
    return;
  }

  const pageRoot = document.querySelector('[data-page-scrollbar]');
  if (pageRoot) {
    apis.push(createOverlayScrollbar(pageRoot, window));
  }

  document.querySelectorAll('[data-scrollbar-scroller]').forEach((scroller) => {
    let local = null;
    if (scroller.parentElement) {
      local = scroller.parentElement.querySelector(':scope > [data-custom-scrollbar]');
    }
    if (!local) {
      local = scroller.querySelector('[data-custom-scrollbar]');
    }
    if (!local) return;
    apis.push(createOverlayScrollbar(local, scroller));
  });
}

function bindViewportSwitch() {
  const queries = ['(max-width: 720px)', '(hover: none) and (pointer: coarse)'];
  for (const q of queries) {
    try {
      const mq = window.matchMedia(q);
      const onChange = () => {
        destroyAll();
        mountPageScrollbars();
      };
      if (typeof mq.addEventListener === 'function') {
        mq.addEventListener('change', onChange);
      } else if (typeof mq.addListener === 'function') {
        mq.addListener(onChange);
      }
    } catch {
      /* ignore */
    }
  }
}

/** Mount overlay scrollbars (skipped on mobile / coarse pointer). */
export function initPageScrollbars() {
  mountPageScrollbars();
  bindViewportSwitch();
}
