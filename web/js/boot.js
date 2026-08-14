import { initTheme } from './theme.js';
import { initPageScrollbars } from './page-scrollbar.js';

/** Shared page boot: theme + custom scrollbars. */
export function bootApp() {
  initTheme();
  initPageScrollbars();
}
