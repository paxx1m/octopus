let lockCount = 0;

/** Marker attribute on nested portal overlays (Esc / click-outside). */
export const PORTAL_OVERLAY_ATTR = 'data-portal-overlay';

/** Nested-safe body scroll lock (ref-counted). */
export function lockBodyScroll() {
    if (typeof document === 'undefined') return;
    lockCount += 1;
    if (lockCount === 1) {
        document.body.classList.add('overflow-hidden');
    }
}

export function unlockBodyScroll() {
    if (typeof document === 'undefined') return;
    if (lockCount <= 0) return;
    lockCount -= 1;
    if (lockCount === 0) {
        document.body.classList.remove('overflow-hidden');
    }
}
