import type { ReactNode } from 'react';
import {
    MorphingDialogClose,
    MorphingDialogDescription,
    MorphingDialogTitle,
} from '@/components/ui/morphing-dialog';
import { cn } from '@/lib/utils';

const sizeClass = {
    sm: 'w-[min(420px,calc(100vw-2rem))]',
    md: 'w-[min(36rem,calc(100vw-2rem))]',
    lg: 'w-[min(56rem,calc(100vw-2rem))]',
    xl: 'w-[min(80vw,calc(100vw-2rem))]',
    full: 'w-[calc(100vw-2rem)]',
} as const;

export type DialogShellSize = keyof typeof sizeClass;

export type DialogShellProps = {
    title: ReactNode;
    children: ReactNode;
    footer?: ReactNode;
    size?: DialogShellSize;
    /** body: header/footer fixed, body scrolls; content: whole shell scrolls */
    scroll?: 'body' | 'content' | 'none';
    className?: string;
    bodyClassName?: string;
    showClose?: boolean;
    closeClassName?: string;
};

export function DialogShell({
    title,
    children,
    footer,
    size = 'md',
    scroll = 'body',
    className,
    bodyClassName,
    showClose = true,
    closeClassName,
}: DialogShellProps) {
    const bodyScroll =
        scroll === 'body'
            ? 'flex-1 min-h-0 overflow-y-auto overscroll-contain'
            : scroll === 'none'
              ? 'flex-1 min-h-0 overflow-hidden'
              : undefined;

    return (
        <div
            className={cn(
                'flex min-h-0 flex-1 flex-col',
                sizeClass[size],
                scroll === 'content' && 'overflow-y-auto overscroll-contain',
                scroll === 'none' && 'h-full overflow-hidden',
                className,
            )}
        >
            <MorphingDialogTitle className="shrink-0">
                <header className="mb-4 flex items-center justify-between gap-3 sm:mb-5">
                    <div className="min-w-0 text-2xl font-bold text-card-foreground">{title}</div>
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

            <MorphingDialogDescription className={cn(bodyScroll, bodyClassName)}>
                {children}
            </MorphingDialogDescription>

            {footer ? <div className="mt-auto shrink-0 pt-4">{footer}</div> : null}
        </div>
    );
}

/** Shared panel classes for MorphingDialogContent shells */
export const dialogContentClass = {
    base: 'relative flex min-h-0 flex-col overflow-hidden rounded-3xl bg-card text-card-foreground custom-shadow',
    maxH: 'max-h-[min(90dvh,calc(100dvh-2rem))]',
    fixedH: 'h-[min(90dvh,calc(100dvh-2rem))]',
} as const;

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
