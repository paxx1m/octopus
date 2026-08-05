import type { ReactNode } from 'react';
import {
    MorphingDialogClose,
    MorphingDialogDescription,
    MorphingDialogTitle,
} from '@/components/ui/morphing-dialog';
import { cn } from '@/lib/utils';

/**
 * Outer panel width — apply only on MorphingDialogContent via dialogPanelClass.
 * Do NOT also put these on DialogShell (double width + padding causes clipping).
 */
const sizeClass = {
    sm: 'w-[min(420px,calc(100vw-2rem))]',
    /** ~max-w-xl, mobile nearly full width */
    md: 'w-[min(36rem,calc(100vw-2rem))]',
    /** ~max-w-4xl */
    lg: 'w-[min(56rem,calc(100vw-2rem))]',
    /** log detail: full-ish on phone, 80vw on desktop */
    xl: 'w-[calc(100vw-2rem)] md:w-[min(80vw,calc(100vw-2rem))]',
    full: 'w-[calc(100vw-2rem)]',
} as const;

export type DialogShellSize = keyof typeof sizeClass;

export type DialogShellProps = {
    title: ReactNode;
    children: ReactNode;
    footer?: ReactNode;
    /**
     * @deprecated Width belongs on MorphingDialogContent (dialogPanelClass).
     * Kept optional only for rare cases where Content is w-fit and shell must size itself.
     * Prefer dialogPanelClass on Content + shell without size.
     */
    size?: DialogShellSize;
    /** body: header fixed, body scrolls; none: fill height, child manages scroll */
    scroll?: 'body' | 'content' | 'none';
    className?: string;
    bodyClassName?: string;
    showClose?: boolean;
    closeClassName?: string;
};

/**
 * Inner chrome for MorphingDialog: title + scrollable body + optional footer.
 * Always w-full min-w-0 so it fits the Content panel (which owns the width).
 */
export function DialogShell({
    title,
    children,
    footer,
    size,
    scroll = 'body',
    className,
    bodyClassName,
    showClose = true,
    closeClassName,
}: DialogShellProps) {
    const bodyScroll =
        scroll === 'body'
            ? 'min-h-0 flex-1 overflow-y-auto overscroll-contain'
            : scroll === 'none'
              ? 'min-h-0 flex-1 overflow-hidden'
              : undefined;

    return (
        <div
            className={cn(
                // Fill Content's content-box; never re-apply outer size (avoids padding double-count clip)
                'flex min-h-0 w-full min-w-0 flex-1 flex-col',
                size && sizeClass[size],
                scroll === 'content' && 'overflow-y-auto overscroll-contain',
                scroll === 'none' && 'h-full overflow-hidden',
                className,
            )}
        >
            <MorphingDialogTitle className="shrink-0">
                <header className="mb-4 flex items-center justify-between gap-3 sm:mb-5">
                    <div className="min-w-0 flex-1 text-2xl font-bold text-card-foreground">
                        {title}
                    </div>
                    {showClose && (
                        <MorphingDialogClose
                            className={cn('relative top-0 right-0 shrink-0', closeClassName)}
                            variants={{
                                initial: { opacity: 0, scale: 0.8 },
                                animate: { opacity: 1, scale: 1 },
                                exit: { opacity: 0, scale: 0.8 },
                            }}
                        />
                    )}
                </header>
            </MorphingDialogTitle>

            <MorphingDialogDescription
                className={cn('min-w-0', bodyScroll, bodyClassName)}
                disableLayoutAnimation
            >
                {children}
            </MorphingDialogDescription>

            {footer ? <div className="mt-auto shrink-0 pt-4">{footer}</div> : null}
        </div>
    );
}

/** Shared panel classes for MorphingDialogContent (owns width + height + chrome). */
export const dialogContentClass = {
    base: 'relative box-border flex min-h-0 flex-col overflow-hidden rounded-3xl bg-card text-card-foreground custom-shadow',
    maxH: 'max-h-[min(90dvh,calc(100dvh-2rem))]',
    fixedH: 'h-[min(90dvh,calc(100dvh-2rem))] max-h-[min(90dvh,calc(100dvh-2rem))]',
} as const;

/** Width + shell styles for MorphingDialogContent only. */
export function dialogPanelClass(
    size: DialogShellSize = 'md',
    opts?: { fixedHeight?: boolean; className?: string },
) {
    return cn(
        dialogContentClass.base,
        sizeClass[size],
        opts?.fixedHeight ? dialogContentClass.fixedH : dialogContentClass.maxH,
        'px-4 py-4 sm:px-6',
        opts?.className,
    );
}
