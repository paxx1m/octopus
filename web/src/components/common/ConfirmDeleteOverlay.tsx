import { AnimatePresence, motion } from 'motion/react';
import { Trash2, X } from 'lucide-react';

/**
 * 删除确认浮层：垃圾桶按钮 + 展开后的确认/取消浮层。
 *
 * 封装 group/Card、setting/APIKey、group/ItemList 中重复的
 * layoutId 动画 + AnimatePresence + 确认/取消按钮模式。
 *
 * @example
 * <ConfirmDeleteOverlay
 *     layoutId={`delete-btn-group-${group.id}`}
 *     isConfirming={confirmDelete}
 *     onShowConfirm={() => setConfirmDelete(true)}
 *     onCancel={() => setConfirmDelete(false)}
 *     onConfirm={() => deleteGroup.mutate(group.id)}
 *     isPending={deleteGroup.isPending}
 *     confirmLabel={t('detail.actions.confirmDelete')}
 * />
 */
export function ConfirmDeleteOverlay({
    layoutId,
    isConfirming,
    onShowConfirm,
    onCancel,
    onConfirm,
    isPending,
    confirmLabel,
    className = '',
}: {
    layoutId: string;
    isConfirming: boolean;
    onShowConfirm: () => void;
    onCancel: () => void;
    onConfirm: () => void;
    isPending: boolean;
    confirmLabel: string;
    /** 触发按钮的额外 className */
    className?: string;
}) {
    return (
        <>
            {!isConfirming && (
                <motion.button
                    layoutId={layoutId}
                    type="button"
                    onClick={onShowConfirm}
                    className={`flex size-8 items-center justify-center rounded-lg bg-destructive/10 text-destructive transition-colors hover:bg-destructive hover:text-destructive-foreground ${className}`}
                >
                    <Trash2 className="size-4" />
                </motion.button>
            )}
            <AnimatePresence>
                {isConfirming && (
                    <motion.div
                        layoutId={layoutId}
                        className="absolute inset-0 flex items-center justify-center gap-2 bg-destructive p-2 rounded-xl"
                        transition={{ type: 'spring', stiffness: 400, damping: 30 }}
                    >
                        <button
                            type="button"
                            onClick={onCancel}
                            className="flex size-8 items-center justify-center rounded-lg bg-destructive-foreground/20 text-destructive-foreground transition-all hover:bg-destructive-foreground/30 active:scale-95"
                        >
                            <X className="size-4" />
                        </button>
                        <button
                            type="button"
                            onClick={onConfirm}
                            disabled={isPending}
                            className="flex-1 h-8 flex items-center justify-center gap-1.5 rounded-lg bg-destructive-foreground text-destructive text-sm font-semibold transition-all hover:bg-destructive-foreground/90 active:scale-[0.98] disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                            <Trash2 className="size-3.5" />
                            {isPending ? '...' : confirmLabel}
                        </button>
                    </motion.div>
                )}
            </AnimatePresence>
        </>
    );
}
