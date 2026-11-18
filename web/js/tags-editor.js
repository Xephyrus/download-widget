/**
 * Artwork tag editor.
 *
 * - Enter / 逗号：加入标签并自动保存
 * - 仅点击 × 移除（整颗芯片不可点删，避免误删）
 * - 移除后短时可撤销
 * - 输入框失焦：若有未提交文字则一并加入
 * - 保存走 /api/folders/:id/tags（JSON）
 */

const MAX_TAGS = 20;
const MAX_LEN = 32;
const SAVE_DEBOUNCE_MS = 220;

function parseTags(raw) {
  return String(raw || '')
    .split(/[,，;；\n\r\t]+/)
    .map((t) => t.replace(/^[#\s\u3000]+|[#\s\u3000]+$/g, '').trim())
    .filter(Boolean);
}

function normalizeTags(tags) {
  const seen = new Set();
  const out = [];
  if (!Array.isArray(tags)) return out;
  for (const raw of tags) {
    let t = String(raw || '')
      .replace(/^[#\s\u3000]+|[#\s\u3000]+$/g, '')
      .trim();
    if (!t) continue;
    const chars = Array.from(t);
    if (chars.length > MAX_LEN) t = chars.slice(0, MAX_LEN).join('');
    const key = t.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(t);
    if (out.length >= MAX_TAGS) break;
  }
  return out;
}

function sameTags(a, b) {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false;
  }
  return true;
}

function folderIdFrom(root) {
  let raw = root.dataset.folderId || root.getAttribute('data-folder-id') || '';
  if (!raw) {
    raw = (location.pathname.match(/^\/artworks\/([^/]+)\/?$/) || [])[1] || '';
  }
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

function apiUrl(folderId) {
  return `/api/folders/${encodeURIComponent(folderId)}/tags`;
}

function setStatus(root, kind, text) {
  const el = root.querySelector('[data-tags-status]');
  if (!el) return;
  el.hidden = !text;
  el.classList.remove('is-ok', 'is-hint', 'is-err');
  if (kind) el.classList.add(kind);
  el.textContent = text || '';
}

function renderChips(root, tags) {
  const host = root.querySelector('[data-tags-chips]');
  if (!host) return;
  host.replaceChildren();
  if (!tags.length) {
    const empty = document.createElement('span');
    empty.className = 'tags-empty tags-empty--inline';
    empty.textContent = '尚未添加标签';
    host.appendChild(empty);
    return;
  }
  for (const tag of tags) {
    const chip = document.createElement('span');
    chip.className = 'tag-chip tag-chip--editable';
    chip.dataset.tag = tag;
    const text = document.createElement('span');
    text.className = 'tag-chip-text';
    text.textContent = tag;
    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'tag-chip-remove';
    remove.dataset.tagRemove = tag;
    remove.setAttribute('aria-label', `移除标签 ${tag}`);
    remove.title = `移除「${tag}」`;
    const icon = document.createElement('span');
    icon.className = 'gg-close';
    icon.setAttribute('aria-hidden', 'true');
    remove.appendChild(icon);
    chip.append(text, remove);
    host.appendChild(chip);
  }
}

function readInitialTags(root) {
  const hidden = root.querySelector('[data-tags-value]');
  if (hidden?.value) return normalizeTags(parseTags(hidden.value));
  const chips = [...root.querySelectorAll('[data-tags-chips] [data-tag]')].map(
    (el) => el.getAttribute('data-tag') || ''
  );
  return normalizeTags(chips);
}

function createController(root) {
  const folderId = folderIdFrom(root);
  const input = root.querySelector('[data-tags-input]');
  let tags = readInitialTags(root);
  let savedSnapshot = tags.slice();
  let dirty = false;
  let saving = false;
  let timer = 0;
  let savedTimer = 0;
  let undoTimer = 0;
  let pendingUndo = null; // { tag }
  let gen = 0;

  const syncHidden = () => {
    const hidden = root.querySelector('[data-tags-value]');
    if (hidden) hidden.value = tags.join(', ');
  };

  const paint = () => {
    syncHidden();
    renderChips(root, tags);
  };

  const markDirty = () => {
    dirty = true;
    root.classList.add('is-dirty');
    root.classList.remove('is-saved');
  };

  const showSaved = () => {
    root.classList.remove('is-dirty');
    root.classList.add('is-saved');
    setStatus(root, 'is-ok', '已保存');
    window.clearTimeout(savedTimer);
    savedTimer = window.setTimeout(() => {
      root.classList.remove('is-saved');
      if (!dirty && !saving) setStatus(root, '', '');
    }, 1400);
  };

  const commitInput = () => {
    if (!input) return false;
    const incoming = normalizeTags(parseTags(input.value));
    if (!incoming.length) return false;
    const next = normalizeTags([...tags, ...incoming]);
    const changed = !sameTags(next, tags);
    tags = next;
    input.value = '';
    if (changed) {
      paint();
      markDirty();
    }
    return changed;
  };

  const scheduleSave = (delay = SAVE_DEBOUNCE_MS) => {
    if (!folderId) {
      setStatus(root, 'is-err', '缺少作品 ID，无法保存');
      return;
    }
    dirty = true;
    window.clearTimeout(timer);
    timer = window.setTimeout(() => {
      void flushSave();
    }, delay);
  };

  const flushSave = async () => {
    if (!folderId) return;
    if (saving) {
      dirty = true;
      return;
    }

    const payload = tags.slice();
    const myGen = ++gen;
    dirty = false;
    saving = true;
    root.classList.add('is-saving');
    setStatus(root, 'is-hint', '保存中…');

    try {
      const res = await fetch(apiUrl(folderId), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json;charset=UTF-8',
          Accept: 'application/json',
        },
        body: JSON.stringify({ tags: payload }),
        cache: 'no-store',
      });
      const body = await res.json().catch(() => null);
      if (!res.ok || body?.success === false) {
        const msg = body?.error || body?.message || `保存失败 (${res.status})`;
        throw new Error(msg);
      }

      // Ignore stale responses if a newer save started.
      if (myGen !== gen) return;

      const serverTags = normalizeTags(
        Array.isArray(body?.data?.tags) ? body.data.tags : payload
      );
      // Keep newer local edits if user changed tags during the request.
      if (!dirty) {
        tags = serverTags;
        savedSnapshot = serverTags.slice();
        paint();
        showSaved();
      }
    } catch (err) {
      if (myGen !== gen) return;
      dirty = true;
      setStatus(root, 'is-err', err?.message || '保存失败，请重试');
    } finally {
      saving = false;
      root.classList.remove('is-saving');
      if (myGen === gen && dirty) scheduleSave(120);
    }
  };

  const addTag = (tag) => {
    const next = normalizeTags([...tags, tag]);
    if (sameTags(next, tags)) return false;
    tags = next;
    paint();
    markDirty();
    scheduleSave();
    return true;
  };

  const clearUndo = () => {
    pendingUndo = null;
    window.clearTimeout(undoTimer);
    const host = root.querySelector('[data-tags-undo]');
    if (host) {
      host.hidden = true;
      host.replaceChildren();
    }
  };

  const showUndo = (tag) => {
    const host = root.querySelector('[data-tags-undo]');
    if (!host) return;
    pendingUndo = { tag };
    host.hidden = false;
    host.replaceChildren();
    const msg = document.createElement('span');
    msg.textContent = `已移除「${tag}」`;
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'tags-undo-btn';
    btn.dataset.tagsUndo = '1';
    btn.textContent = '撤销';
    host.append(msg, btn);
    window.clearTimeout(undoTimer);
    undoTimer = window.setTimeout(() => {
      clearUndo();
    }, 5000);
  };

  const removeTag = (tag) => {
    const key = String(tag || '').toLowerCase();
    const hit = tags.find((t) => t.toLowerCase() === key);
    if (!hit) return false;
    const next = tags.filter((t) => t.toLowerCase() !== key);
    tags = next;
    paint();
    markDirty();
    scheduleSave();
    showUndo(hit);
    return true;
  };

  const undoRemove = () => {
    if (!pendingUndo?.tag) return false;
    const tag = pendingUndo.tag;
    clearUndo();
    return addTag(tag);
  };

  // Ensure undo tray exists even if SSR HTML is from an older binary.
  if (!root.querySelector('[data-tags-undo]')) {
    const tray = document.createElement('div');
    tray.className = 'tags-undo';
    tray.setAttribute('data-tags-undo', '');
    tray.hidden = true;
    const chips = root.querySelector('[data-tags-chips]');
    chips?.insertAdjacentElement('afterend', tray);
  }
  const desc = root.querySelector('.tags-panel-desc');
  if (desc) desc.textContent = '回车添加，点 × 移除；变更会自动保存。';

  paint();

  root.addEventListener('click', (e) => {
    const undoBtn = e.target.closest('[data-tags-undo]');
    if (undoBtn && root.contains(undoBtn)) {
      e.preventDefault();
      undoRemove();
      input?.focus();
      return;
    }

    // Only the × button removes — clicking the chip label does nothing.
    const removeBtn = e.target.closest('[data-tag-remove]');
    if (!removeBtn || !root.contains(removeBtn)) return;
    e.preventDefault();
    e.stopPropagation();
    removeTag(removeBtn.getAttribute('data-tag-remove') || '');
    input?.focus();
  });

  if (input) {
    const tryCommit = () => {
      if (commitInput()) {
        clearUndo();
        scheduleSave();
      }
    };

    input.addEventListener('keydown', (e) => {
      // IME: ignore the Enter that confirms composition.
      if (e.isComposing || e.keyCode === 229) return;
      if (e.key === 'Enter' || e.key === ',') {
        e.preventDefault();
        e.stopPropagation();
        tryCommit();
      }
      // Intentionally no Backspace→delete-last-tag (too easy to mis-delete).
    });

    input.addEventListener('input', () => {
      if (/[,，;；]/.test(input.value)) tryCommit();
    });

    input.addEventListener('blur', () => {
      tryCommit();
    });
  }

  // Expose tiny helpers for tests / debugging.
  root._tagsController = {
    getTags: () => tags.slice(),
    getSaved: () => savedSnapshot.slice(),
    addTag,
    removeTag,
    undoRemove,
    saveNow: () => {
      if (commitInput()) {
        /* flushed below */
      }
      scheduleSave(0);
    },
  };

  return root._tagsController;
}

export function mountTagsEditor(root) {
  if (!(root instanceof HTMLElement)) return;
  if (root.dataset.tagsReady === '1') return;
  if (!root.querySelector('[data-tags-input], [data-tags-chips]')) return;
  root.dataset.tagsReady = '1';
  createController(root);

  // Do not autofocus — reduces accidental edits when opening the page.
}

export function mountAllTagsEditors(scope = document) {
  scope.querySelectorAll('[data-tags-editor]').forEach((el) => mountTagsEditor(el));
}
