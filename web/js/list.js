import { bootApp } from './boot.js?v=35';

bootApp();

const FILTER_KEYS = [
  'search',
  'tag',
  'pageSize',
  'sortBy',
  'sortOrder',
  'page',
  'minFileCount',
  'maxFileCount',
  'exactMatch',
  'startDate',
  'endDate',
];

const FILTER_DEFAULTS = {
  pageSize: '20',
  sortBy: 'createdAt',
  sortOrder: 'desc',
  page: '1',
};

function canonicalListPath(sp) {
  const out = new URLSearchParams();
  for (const key of FILTER_KEYS) {
    const raw = sp.get(key);
    if (raw == null) continue;
    const v = String(raw).trim();
    if (!v) continue;
    if (FILTER_DEFAULTS[key] && v === FILTER_DEFAULTS[key]) continue;
    out.set(key, v);
  }
  const q = out.toString();
  return q ? `/?${q}` : '/';
}

function paramsFromForm(form) {
  const fd = new FormData(form);
  const sp = new URLSearchParams();
  for (const key of FILTER_KEYS) {
    const v = String(fd.get(key) ?? '').trim();
    if (v) sp.set(key, v);
  }
  return sp;
}

function replaceListUrl(sp) {
  const next = canonicalListPath(sp);
  const cur = `${location.pathname}${location.search}`;
  if (cur === next) return;
  history.replaceState(history.state && typeof history.state === 'object' ? { ...history.state, htmx: true } : { htmx: true }, '', next);
}

function hydrateFiltersFromUrl() {
  const form = document.getElementById('listFilters');
  if (!form || !location.search) return;

  const sp = new URLSearchParams(location.search);
  const setVal = (selector, key) => {
    const el = form.querySelector(selector);
    if (!el || !sp.has(key)) return;
    el.value = sp.get(key) || '';
  };

  setVal('#searchInput', 'search');
  setVal('#tagFilter', 'tag');
  setVal('#pageSizeSelect', 'pageSize');
  setVal('#sortBySelect', 'sortBy');
  setVal('#sortOrderSelect', 'sortOrder');
  setVal('#minFileCount', 'minFileCount');
  setVal('#maxFileCount', 'maxFileCount');
  setVal('input[name="page"]', 'page');
}

function resetPageToFirst() {
  const pageEl = document.querySelector('#listFilters input[name="page"]');
  if (pageEl) pageEl.value = '1';
}

function requestUrlFromDetail(detail) {
  const candidates = [
    detail?.request?.url,
    detail?.request?.action,
    detail?.ctx?.request?.action,
    detail?.ctx?.request?.url,
    detail?.pathInfo?.requestPath,
    detail?.xhr?.responseURL,
  ];
  for (const c of candidates) {
    if (!c) continue;
    try {
      return new URL(String(c), location.origin);
    } catch {
      /* ignore */
    }
  }
  return null;
}

function syncUrlAfterListRequest(detail) {
  const u = requestUrlFromDetail(detail);
  if (u && u.pathname.includes('/partials/list')) {
    replaceListUrl(u.searchParams);
    const page = u.searchParams.get('page') || '1';
    const pageEl = document.querySelector('#listFilters input[name="page"]');
    if (pageEl) pageEl.value = page;
    return;
  }
  const form = document.getElementById('listFilters');
  if (form) replaceListUrl(paramsFromForm(form));
}

const gallery = document.querySelector('a[data-nav="gallery"]');
if (gallery) {
  gallery.addEventListener('click', (e) => {
    if (location.pathname === '/' || location.pathname === '') {
      e.preventDefault();
    }
  });
}

async function loadAllTags() {
  const list = document.getElementById('allTagsList');
  if (!list) return;
  try {
    const res = await fetch('/api/tags');
    const data = await res.json();
    const tags = data?.data?.tags ?? data?.tags ?? [];
    if (!Array.isArray(tags)) return;
    list.innerHTML = '';
    for (const tag of tags) {
      const opt = document.createElement('option');
      opt.value = String(tag);
      list.appendChild(opt);
    }
  } catch {
    /* ignore */
  }
}

const form = document.getElementById('listFilters');
if (form) {
  // Changing filters should jump back to page 1.
  form.addEventListener(
    'change',
    (e) => {
      if (e.target && e.target.name && e.target.name !== 'page') resetPageToFirst();
    },
    true
  );
  form.querySelector('#searchInput')?.addEventListener('input', () => resetPageToFirst());
  form.querySelector('#tagFilter')?.addEventListener('input', () => resetPageToFirst());

  document.getElementById('resetFilters')?.addEventListener('click', () => {
    // After native reset() from hx-on:click, force defaults into URL soon after.
    window.setTimeout(() => {
      replaceListUrl(new URLSearchParams());
      const pageEl = form.querySelector('input[name="page"]');
      if (pageEl) pageEl.value = '1';
    }, 0);
  });
}

document.body.addEventListener('htmx:after:request', (e) => {
  const detail = e.detail;
  const status = detail?.ctx?.response?.status ?? detail?.xhr?.status;
  if (typeof status === 'number' && status >= 400) return;
  syncUrlAfterListRequest(detail);
});

document.body.addEventListener('htmx:after:swap', (e) => {
  if (e.detail?.target?.id === 'tableBody') {
    loadAllTags();
  }
});

hydrateFiltersFromUrl();
loadAllTags();
