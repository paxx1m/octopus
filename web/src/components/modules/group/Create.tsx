import type { GroupItem } from '@/api/endpoints/group';
import { useMorphingDialog } from '@/components/ui/morphing-dialog';
import { DialogShell } from '@/components/common/DialogShell';
import { useCreateGroup } from '@/api/endpoints/group';
import { useTranslations } from 'use-intl';
import { GroupEditor } from './Editor';
import { toast } from '@/components/common/Toast';

export function CreateDialogContent() {
    const { setIsOpen } = useMorphingDialog();
    const createGroup = useCreateGroup();
    const t = useTranslations('group');

    return (
        <DialogShell title={t('create.title')} scroll="none">
            <GroupEditor
                submitText={t('create.submit')}
                submittingText={t('create.submitting')}
                isSubmitting={createGroup.isPending}
                onSubmit={({ name, match_regex, mode, first_token_time_out, session_keep_time, members }) => {
                    const items: GroupItem[] = members.map((member, index) => ({
                        channel_id: member.channel_id,
                        model_name: member.name,
                        priority: index + 1,
                        weight: member.weight ?? 1,
                    }));

                    createGroup.mutate(
                        {
                            name,
                            mode,
                            match_regex: match_regex ?? '',
                            first_token_time_out: first_token_time_out ?? 0,
                            session_keep_time: session_keep_time ?? 0,
                            items,
                        },
                        {
                            onSuccess: () => setIsOpen(false),
                            onError: (error) =>
                                toast.error(t('toast.createFailed'), { description: error.message }),
                        },
                    );
                }}
            />
        </DialogShell>
    );
}
