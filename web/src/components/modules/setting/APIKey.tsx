import { useCallback, useId, useMemo, useState, type ReactNode } from 'react';
import { useTranslations } from 'use-intl';
import { KeyRound, Plus, Loader, Trash2, X, Info, Pencil, Maximize2 } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import {
    MorphingDialog,
    MorphingDialogContainer,
    MorphingDialogContent,
    MorphingDialogTrigger,
    useMorphingDialog,
} from '@/components/ui/morphing-dialog';
import { dialogPanelClass } from '@/components/common/DialogShell';
import {
    useAPIKeyList,
    useCreateAPIKey,
    useUpdateAPIKey,
    useDeleteAPIKey,
    type APIKey,
    type APIKeyFormData,
} from '@/api/endpoints/apikey';
import { useStatsAPIKey } from '@/api/endpoints/stats';
import { toast } from '@/components/common/Toast';
import { CopyIconButton } from '@/components/common/CopyButton';
import { PortalOverlay } from '@/components/common/PortalOverlay';
import type { ApiError } from '@/api/types';
import { APIKeyFormOverlay } from './APIKeyForm';

function APIKeyStatsCard({
    open,
    layoutId,
    apiKey,
    onClose,
}: {
    open: boolean;
    layoutId: string;
    apiKey: APIKey | null;
    onClose: () => void;
}) {
    const t = useTranslations('setting');
    const { data: statsList = [] } = useStatsAPIKey();
    const stats = useMemo(
        () => (apiKey ? statsList.find((s) => s.api_key_id === apiKey.id) : undefined),
        [statsList, apiKey],
    );

    return (
        <PortalOverlay
            open={open}
            onClose={onClose}
            layoutId={layoutId}
            className="w-[min(320px,calc(100vw-2rem))]"
        >
            {apiKey ? (
                <>
                    <div className="mb-3 flex items-center justify-between gap-2">
                        <h3 className="line-clamp-1 text-sm font-semibold text-card-foreground">
                            {apiKey.name}
                        </h3>
                        <button
                            type="button"
                            onClick={onClose}
                            className="flex size-8 items-center justify-center rounded-lg bg-muted text-muted-foreground transition-colors hover:bg-muted/80"
                        >
                            <X className="size-4" />
                        </button>
                    </div>

                    {!stats ? (
                        <div className="text-sm text-muted-foreground">{t('apiKey.stats.noData')}</div>
                    ) : (
                        <div className="grid grid-cols-2 gap-3 text-sm">
                            <div className="rounded-lg bg-muted/40 p-3">
                                <div className="text-xs text-muted-foreground">{t('apiKey.stats.inputToken')}</div>
                                <div className="font-medium tabular-nums">
                                    {stats.input_token.formatted.value}
                                    {stats.input_token.formatted.unit}
                                </div>
                            </div>
                            <div className="rounded-lg bg-muted/40 p-3">
                                <div className="text-xs text-muted-foreground">{t('apiKey.stats.outputToken')}</div>
                                <div className="font-medium tabular-nums">
                                    {stats.output_token.formatted.value}
                                    {stats.output_token.formatted.unit}
                                </div>
                            </div>
                            <div className="rounded-lg bg-muted/40 p-3">
                                <div className="text-xs text-muted-foreground">{t('apiKey.stats.inputCost')}</div>
                                <div className="font-medium tabular-nums">
                                    {stats.input_cost.formatted.value}
                                    {stats.input_cost.formatted.unit}
                                </div>
                            </div>
                            <div className="rounded-lg bg-muted/40 p-3">
                                <div className="text-xs text-muted-foreground">{t('apiKey.stats.outputCost')}</div>
                                <div className="font-medium tabular-nums">
                                    {stats.output_cost.formatted.value}
                                    {stats.output_cost.formatted.unit}
                                </div>
                            </div>
                            <div className="rounded-lg bg-muted/40 p-3">
                                <div className="text-xs text-muted-foreground">{t('apiKey.stats.requestSuccess')}</div>
                                <div className="font-medium tabular-nums">
                                    {stats.request_success.formatted.value}
                                    {stats.request_success.formatted.unit}
                                </div>
                            </div>
                            <div className="rounded-lg bg-muted/40 p-3">
                                <div className="text-xs text-muted-foreground">{t('apiKey.stats.requestFailed')}</div>
                                <div className="font-medium tabular-nums">
                                    {stats.request_failed.formatted.value}
                                    {stats.request_failed.formatted.unit}
                                </div>
                            </div>
                        </div>
                    )}
                </>
            ) : null}
        </PortalOverlay>
    );
}

function APIKeyKeyItem({
    apiKey,
    statsLayoutId,
    editLayoutId,
    deleteLayoutId,
    onViewStats,
    onEdit,
    onDelete,
    isDeleting,
}: {
    apiKey: APIKey;
    statsLayoutId: string;
    editLayoutId: string;
    deleteLayoutId: string;
    onViewStats: () => void;
    onEdit: () => void;
    onDelete: () => void;
    isDeleting: boolean;
}) {
    const t = useTranslations('setting');
    const [confirmDelete, setConfirmDelete] = useState(false);

    return (
        <motion.div
            layout
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.95, transition: { duration: 0.2 } }}
            transition={{ type: 'spring', stiffness: 500, damping: 30 }}
            className="group relative flex items-center justify-between gap-3 p-3 rounded-xl bg-muted/50 overflow-hidden origin-top"
        >
            <span className="text-sm font-medium truncate">{apiKey.name}</span>

            <div className="flex items-center gap-1.5">
                <motion.button
                    type="button"
                    layoutId={statsLayoutId}
                    onClick={onViewStats}
                    className="flex size-8 items-center justify-center rounded-lg bg-muted/60 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground active:scale-95"
                    title="Stats"
                >
                    <Info className="size-4" />
                </motion.button>
                <motion.button
                    type="button"
                    layoutId={editLayoutId}
                    onClick={onEdit}
                    className="flex size-8 items-center justify-center rounded-lg bg-muted/60 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground active:scale-95"
                    title="Edit"
                >
                    <Pencil className="size-4" />
                </motion.button>
                <CopyIconButton
                    text={apiKey.api_key}
                    className="flex size-8 items-center justify-center rounded-lg bg-primary/10 text-primary transition-all hover:bg-primary hover:text-primary-foreground active:scale-95"
                    copyIconClassName="size-4"
                    checkIconClassName="size-4"
                />

                {!confirmDelete && (
                    <motion.button
                        layoutId={deleteLayoutId}
                        onClick={() => setConfirmDelete(true)}
                        className="flex size-8 items-center justify-center rounded-lg bg-destructive/10 text-destructive transition-colors hover:bg-destructive hover:text-destructive-foreground"
                    >
                        <Trash2 className="size-4" />
                    </motion.button>
                )}
            </div>

            <AnimatePresence>
                {confirmDelete && (
                    <motion.div
                        layoutId={deleteLayoutId}
                        className="absolute inset-0 flex items-center justify-center gap-2 bg-destructive p-3 rounded-xl"
                        transition={{ type: 'spring', stiffness: 400, damping: 30 }}
                    >
                        <button
                            onClick={() => setConfirmDelete(false)}
                            className="flex size-8 items-center justify-center rounded-lg bg-destructive-foreground/20 text-destructive-foreground transition-all hover:bg-destructive-foreground/30 active:scale-95"
                        >
                            <X className="size-4" />
                        </button>
                        <button
                            onClick={onDelete}
                            disabled={isDeleting}
                            className="flex-1 h-8 flex items-center justify-center gap-1.5 rounded-lg bg-destructive-foreground text-destructive text-sm font-medium transition-all hover:bg-destructive-foreground/90 active:scale-[0.98] disabled:opacity-50"
                        >
                            <Trash2 className="size-3.5" />
                            {isDeleting ? '...' : t('apiKey.form.confirm')}
                        </button>
                    </motion.div>
                )}
            </AnimatePresence>
        </motion.div>
    );
}

function APIKeyPanelBase({
    idPrefix,
    containerClassName,
    listClassName,
    renderHeaderExtra,
}: {
    idPrefix: string;
    containerClassName: string;
    listClassName: string;
    renderHeaderExtra?: (ctx: {
        disabled: boolean;
        onCloseAllOverlays: () => void;
    }) => ReactNode;
}) {
    const t = useTranslations('setting');
    const { data: apiKeys, isLoading: apiKeysLoading, error: apiKeysError } = useAPIKeyList();
    const createAPIKey = useCreateAPIKey();
    const updateAPIKey = useUpdateAPIKey();
    const deleteAPIKey = useDeleteAPIKey();

    const instanceId = useId();
    const addLayoutId = `add-btn-${idPrefix}-${instanceId}`;
    const statsPrefix = `${idPrefix}-stats-${instanceId}`;
    const editPrefix = `${idPrefix}-edit-${instanceId}`;
    const deletePrefix = `${idPrefix}-delete-`;

    const [isAdding, setIsAdding] = useState(false);
    const [viewingStats, setViewingStats] = useState<{ apiKey: APIKey; layoutId: string } | null>(null);
    const [editingKey, setEditingKey] = useState<{ apiKey: APIKey; layoutId: string } | null>(null);
    const [deletingId, setDeletingId] = useState<number | null>(null);

    const sortedApiKeys = useMemo(() => {
        if (!apiKeys) return [];
        return [...apiKeys].sort((a, b) => a.id - b.id);
    }, [apiKeys]);

    const handleDelete = useCallback((id: number) => {
        setDeletingId(id);
        deleteAPIKey.mutate(id, {
            onSuccess: () => {
                toast.success(t('apiKey.toast.deleteSuccess'));
            },
            onError: (error) => {
                const msg = (error as unknown as ApiError)?.message;
                toast.error(t('apiKey.toast.deleteError'), { description: msg });
            },
            onSettled: () => setDeletingId((cur) => (cur === id ? null : cur)),
        });
    }, [deleteAPIKey, t]);

    const closeAllOverlays = useCallback(() => {
        setIsAdding(false);
        setViewingStats(null);
        setEditingKey(null);
    }, []);

    const disabledHeaderActions = createAPIKey.isPending || isAdding || !!viewingStats || !!editingKey;

    const handleCreate = useCallback((data: APIKeyFormData) => {
        createAPIKey.mutate(data, {
            onSuccess: () => {
                toast.success(t('apiKey.toast.createSuccess'));
                setIsAdding(false);
            },
            onError: (error) => {
                const msg = (error as unknown as ApiError)?.message;
                toast.error(t('apiKey.toast.createError'), { description: msg });
            },
        });
    }, [createAPIKey, t]);

    const handleUpdate = useCallback((apiKey: APIKey, data: APIKeyFormData) => {
        updateAPIKey.mutate({ id: apiKey.id, ...data }, {
            onSuccess: () => {
                toast.success(t('apiKey.toast.updateSuccess'));
                setEditingKey(null);
            },
            onError: (error) => {
                const msg = (error as unknown as ApiError)?.message;
                toast.error(t('apiKey.toast.updateError'), { description: msg });
            },
        });
    }, [t, updateAPIKey]);

    return (
        <div className={containerClassName}>
            <div className="flex items-center justify-between gap-3">
                <h2 className="text-lg font-bold text-card-foreground flex items-center gap-2">
                    <KeyRound className="h-5 w-5" />
                    {t('apiKey.title')}
                </h2>
                <div className="flex items-center gap-2">
                    <motion.button
                        layoutId={addLayoutId}
                        type="button"
                        onClick={() => setIsAdding(true)}
                        disabled={disabledHeaderActions}
                        className="h-9 w-9 flex items-center justify-center rounded-lg bg-muted/60 text-muted-foreground transition-colors hover:bg-muted disabled:opacity-50"
                        title={t('apiKey.add')}
                    >
                        <Plus className="size-4" />
                    </motion.button>
                    {renderHeaderExtra?.({ disabled: disabledHeaderActions, onCloseAllOverlays: closeAllOverlays })}
                </div>
            </div>

            <APIKeyFormOverlay
                open={isAdding}
                layoutId={addLayoutId}
                isPending={createAPIKey.isPending}
                submitLabel={t('apiKey.form.create')}
                onSubmit={handleCreate}
                onClose={() => setIsAdding(false)}
            />

            <APIKeyStatsCard
                open={!!viewingStats}
                layoutId={viewingStats?.layoutId ?? `${statsPrefix}-idle`}
                apiKey={viewingStats?.apiKey ?? null}
                onClose={() => setViewingStats(null)}
            />

            <APIKeyFormOverlay
                open={!!editingKey}
                layoutId={editingKey?.layoutId ?? `${editPrefix}-idle`}
                apiKey={editingKey?.apiKey}
                isPending={updateAPIKey.isPending}
                submitLabel={t('apiKey.form.save')}
                onSubmit={(data) => {
                    if (!editingKey) return;
                    handleUpdate(editingKey.apiKey, data);
                }}
                onClose={() => setEditingKey(null)}
            />

            <div className={listClassName}>
                {apiKeysLoading ? (
                    <div className="h-full flex items-center justify-center text-sm text-muted-foreground">
                        <Loader className="size-4 animate-spin" />
                    </div>
                ) : apiKeysError ? (
                    <div className="h-full flex items-center justify-center text-sm text-destructive">
                        {t('apiKey.loadFailed')}
                    </div>
                ) : apiKeys?.length === 0 ? (
                    <div className="h-full flex items-center justify-center text-sm text-muted-foreground">
                        {t('apiKey.empty')}
                    </div>
                ) : (
                    <AnimatePresence>
                        {sortedApiKeys.map((apiKey) => {
                            const statsLayoutId = `${statsPrefix}-${apiKey.id}`;
                            const editLayoutId = `${editPrefix}-${apiKey.id}`;
                            const deleteLayoutId = `${deletePrefix}${apiKey.id}`;
                            return (
                                <APIKeyKeyItem
                                    key={apiKey.id}
                                    apiKey={apiKey}
                                    statsLayoutId={statsLayoutId}
                                    editLayoutId={editLayoutId}
                                    deleteLayoutId={deleteLayoutId}
                                    onViewStats={() => {
                                        closeAllOverlays();
                                        setViewingStats({ apiKey, layoutId: statsLayoutId });
                                    }}
                                    onEdit={() => {
                                        closeAllOverlays();
                                        setEditingKey({ apiKey, layoutId: editLayoutId });
                                    }}
                                    onDelete={() => handleDelete(apiKey.id)}
                                    isDeleting={deleteAPIKey.isPending && deletingId === apiKey.id}
                                />
                            );
                        })}
                    </AnimatePresence>
                )}
            </div>
        </div>
    );
}

function APIKeyDialogPanel() {
    const { setIsOpen } = useMorphingDialog();
    return (
        <APIKeyPanelBase
            idPrefix="apikey-dialog"
            containerClassName="relative w-full space-y-5"
            listClassName="h-[min(70dvh,calc(100dvh-10rem))] space-y-2 overflow-y-auto"
            renderHeaderExtra={() => (
                <button
                    type="button"
                    onClick={() => setIsOpen(false)}
                    className="h-9 w-9 flex items-center justify-center rounded-lg bg-muted/60 text-muted-foreground transition-colors hover:bg-muted"
                    title="Close"
                >
                    <X className="size-4" />
                </button>
            )}
        />
    );
}

export function SettingAPIKey() {
    return (
        <APIKeyPanelBase
            idPrefix="apikey"
            containerClassName="rounded-3xl border border-border bg-card p-6 space-y-5 relative"
            listClassName="space-y-2 h-36 overflow-y-auto"
            renderHeaderExtra={() => (
                <MorphingDialog>
                    <MorphingDialogTrigger className="h-9 w-9 flex items-center justify-center rounded-lg bg-muted/60 text-muted-foreground transition-colors hover:bg-muted">
                        <Maximize2 className="size-4" />
                    </MorphingDialogTrigger>
                    <MorphingDialogContainer>
                        <MorphingDialogContent className={dialogPanelClass('md')}>
                            <APIKeyDialogPanel />
                        </MorphingDialogContent>
                    </MorphingDialogContainer>
                </MorphingDialog>
            )}
        />
    );
}
