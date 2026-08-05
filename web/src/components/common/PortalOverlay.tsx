import { useEffect, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { AnimatePresence, motion } from 'motion/react';
import { cn } from '@/lib/utils';
import { lockBodyScroll, unlockBodyScroll, PORTAL_OVERLAY_ATTR } from '@/lib/body-scroll-lock';

type PortalOverlayProps = {
    open: boolean;
    onClose: () => void;
    children: ReactNode;
    /** layoutId for shared-element morph from trigger */
    layoutId?: string;
    className?: string;
    /** lock body scroll while open */
    lockScroll?: boolean;
};

/**
 * Fixed centered overlay via portal — avoids parent overflow clipping on mobile.
 * Nested-safe with MorphingDialog (body lock ref-count, Esc stops at top layer).
 */
export function PortalOverlay({
    open,
    onClose,
    children,
    layoutId,
    className,
    lockScroll = true,
}: PortalOverlayProps) {
    const [mounted, setMounted] = useState(false);

    useEffect(() => {
        setMounted(true);
    }, []);

    useEffect(() => {
        if (!open || !lockScroll) return;
        lockBodyScroll();
        return () => unlockBodyScroll();
    }, [open, lockScroll]);

    useEffect(() => {
        if (!open) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== 'Escape') return;
            e.stopPropagation();
            e.preventDefault();
            onClose();
        };
        document.addEventListener('keydown', onKey, true);
        return () => document.removeEventListener('keydown', onKey, true);
    }, [open, onClose]);

    if (!mounted) return null;

    return createPortal(
        <AnimatePresence>
            {open ? (
                <motion.div
                    key="portal-overlay-root"
                    {...{ [PORTAL_OVERLAY_ATTR]: '' }}
                    className="fixed inset-0 z-[60] flex items-center justify-center p-4"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                >
                    <div
                        className="absolute inset-0 bg-white/40 backdrop-blur-xs dark:bg-black/40"
                        onClick={onClose}
                        aria-hidden
                    />
                    <motion.div
                        layoutId={layoutId}
                        className={cn(
                            'relative z-10 max-h-[min(80dvh,calc(100dvh-2rem))] w-[min(420px,calc(100vw-2rem))] overflow-y-auto overscroll-contain rounded-3xl border border-border bg-card p-5',
                            className,
                        )}
                        initial={{ opacity: 0, scale: 0.96 }}
                        animate={{ opacity: 1, scale: 1 }}
                        exit={{ opacity: 0, scale: 0.96 }}
                        transition={{ type: 'spring', stiffness: 400, damping: 30 }}
                        onClick={(e) => e.stopPropagation()}
                    >
                        {children}
                    </motion.div>
                </motion.div>
            ) : null}
        </AnimatePresence>,
        document.body,
    );
}
