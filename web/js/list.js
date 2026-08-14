import { bootApp } from './boot.js';

bootApp();

const gallery = document.querySelector('a[data-nav="gallery"]');
if (gallery) {
  gallery.addEventListener('click', (e) => {
    if (location.pathname === '/' || location.pathname === '') {
      e.preventDefault();
    }
  });
}
