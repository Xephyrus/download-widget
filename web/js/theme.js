const KEY = 'pixiv-shelf-theme';

function readStored() {
  try {
    const saved = localStorage.getItem(KEY);
    if (saved === 'light' || saved === 'dark') return saved;
  } catch {
    /* ignore */
  }
  return null;
}

function systemTheme() {
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
}

export function currentTheme() {
  return document.documentElement.getAttribute('data-theme') || readStored() || systemTheme();
}

function apply(theme) {
  document.documentElement.setAttribute('data-theme', theme);
  const btn = document.getElementById('themeToggle');
  if (!btn) return;
  const toDark = theme === 'light';
  btn.setAttribute('aria-label', toDark ? '切换到深色主题' : '切换到浅色主题');
  btn.setAttribute('title', toDark ? '浅色 · 点击切换深色' : '深色 · 点击切换浅色');
}

function save(theme) {
  try {
    localStorage.setItem(KEY, theme);
  } catch {
    /* ignore */
  }
}

export function setTheme(theme) {
  if (theme !== 'light' && theme !== 'dark') return;
  apply(theme);
  save(theme);
}

export function toggleTheme() {
  const next = currentTheme() === 'light' ? 'dark' : 'light';
  setTheme(next);
  return next;
}

/** Apply theme and bind #themeToggle. */
export function initTheme() {
  apply(readStored() || systemTheme());
  const btn = document.getElementById('themeToggle');
  if (btn && !btn.dataset.themeBound) {
    btn.dataset.themeBound = '1';
    btn.addEventListener('click', () => {
      toggleTheme();
    });
  }
}
