import { useEffect, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { motion } from 'motion/react';
import { cn } from '@/lib/utils';

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
 * Optional layoutId keeps morph animation with motion triggers.
 */
export function PortalOverlay({
    open,
    onClose,
    children,
    layoutId,
    className,
    lockScroll = true,
}: PortalOverlayProps) {
    useEffect(() => {
        if (!open || !lockScroll) return;
        document.body.classList.add('overflow-hidden');
        return () => {
            document.body.classList.remove('overflow-hidden');
        };
    }, [open, lockScroll]);

    useEffect(() => {
        if (!open) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') onClose();
        };
        document.addEventListener('keydown', onKey);
        return () => document.removeEventListener('keydown', onKey);
    }, [open, onClose]);

    if (!open || typeof document === 'undefined') return null;

    return createPortal(
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
            <motion.div
                className="absolute inset-0 bg-white/40 backdrop-blur-xs dark:bg-black/40"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                onClick={onClose}
            />
            <motion.div
                layoutId={layoutId}
                className={cn(
                    'relative z-10 max-h-[min(80dvh,calc(100dvh-2rem))] w-[min(420px,calc(100vw-2rem))] overflow-y-auto overscroll-contain rounded-3xl border border-border bg-card p-5',
                    className,
                )}
                transition={{ type: 'spring', stiffness: 400, damping: 30 }}
                onClick={(e) => e.stopPropagation()}
            >
                {children}
            </motion.div>
        </div>,
        document.body,
    );
}
