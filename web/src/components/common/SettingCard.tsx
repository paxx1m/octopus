import type { ReactNode } from 'react';

/**
 * 设置页通用卡片壳：统一 rounded-3xl border bg-card p-6 + 带图标的标题行。
 */
export function SettingCard({ icon, title, children }: { icon: ReactNode; title: string; children: ReactNode }) {
    return (
        <div className="rounded-3xl border border-border bg-card p-6 space-y-5">
            <h2 className="text-lg font-bold text-card-foreground flex items-center gap-2">
                {icon}
                {title}
            </h2>
            {children}
        </div>
    );
}

/**
 * 设置页通用行：左侧 icon + label（可选 tooltip），右侧 children（Input/Switch/Button）。
 */
export function SettingRow({
    icon,
    label,
    children,
}: {
    icon?: ReactNode;
    label: ReactNode;
    children: ReactNode;
}) {
    return (
        <div className="flex items-center justify-between gap-4">
            <div className="flex items-center gap-3">
                {icon}
                <span className="text-sm font-medium">{label}</span>
            </div>
            {children}
        </div>
    );
}
