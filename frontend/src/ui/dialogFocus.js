// Shared keyboard and scroll behavior for the existing dialog templates.
const stack = [];
const records = new WeakMap();
let savedOverflow = '';
let savedPadding = '';
let savedOverscroll = '';
const selector = 'button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [contenteditable="true"], audio[controls], video[controls], [tabindex]:not([tabindex="-1"])';

function focusable(panel) {
  return [...panel.querySelectorAll(selector)].filter((el) => el.getClientRects().length && !el.closest('[inert]'));
}

export const vDialogFocus = {
  mounted(panel, binding) {
    const record = { panel, close: binding.value, previous: document.activeElement };
    records.set(panel, record);
    if (!stack.length) {
      savedOverflow = document.body.style.overflow;
      savedPadding = document.body.style.paddingRight;
      savedOverscroll = document.body.style.overscrollBehavior;
      const gutter = window.innerWidth - document.documentElement.clientWidth;
      if (gutter > 0) document.body.style.paddingRight = `${gutter}px`;
      document.body.style.overflow = 'hidden';
      document.body.style.overscrollBehavior = 'none';
    }
    stack.push(record);
    if (!panel.hasAttribute('role')) panel.setAttribute('role', 'dialog');
    if (!panel.hasAttribute('aria-modal')) panel.setAttribute('aria-modal', 'true');
    panel.setAttribute('tabindex', '-1');
    if (!panel.hasAttribute('aria-label') && !panel.hasAttribute('aria-labelledby')) {
      panel.setAttribute('aria-label', panel.querySelector('h2, h3')?.textContent?.trim() || '对话窗口');
    }
    record.onKey = (event) => {
      if (stack.slice(-1)[0] !== record) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        record.close?.();
      } else if (event.key === 'Tab') {
        const items = focusable(panel);
        const first = items[0];
        const last = items.slice(-1)[0];
        if (!first) { event.preventDefault(); panel.focus(); }
        else if (event.shiftKey && (document.activeElement === first || !items.includes(document.activeElement))) {
          event.preventDefault(); last.focus();
        } else if (!event.shiftKey && (document.activeElement === last || !items.includes(document.activeElement))) {
          event.preventDefault(); first.focus();
        }
      }
    };
    document.addEventListener('keydown', record.onKey, true);
    queueMicrotask(() => {
      if (stack.slice(-1)[0] === record && panel.isConnected) {
        (panel.querySelector('[data-dialog-autofocus]') || focusable(panel)[0] || panel).focus();
      }
    });
  },
  updated(panel, binding) {
    const record = records.get(panel);
    if (record) record.close = binding.value;
  },
  unmounted(panel) {
    const record = records.get(panel);
    if (!record) return;
    document.removeEventListener('keydown', record.onKey, true);
    const wasTop = stack.slice(-1)[0] === record;
    const index = stack.indexOf(record);
    if (index >= 0) stack.splice(index, 1);
    if (!stack.length) {
      document.body.style.overflow = savedOverflow;
      document.body.style.paddingRight = savedPadding;
      document.body.style.overscrollBehavior = savedOverscroll;
    }
    if (wasTop) {
      const parent = stack.slice(-1)[0]?.panel;
      if (record.previous?.isConnected && (!parent || parent.contains(record.previous))) record.previous.focus();
      else parent?.focus();
    }
    records.delete(panel);
  },
};
