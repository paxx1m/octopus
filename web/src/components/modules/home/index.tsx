import { Activity } from './activity';
import { Total } from './total';
import { StatsChart } from './chart';
import { ChannelRanking } from './channel-ranking';
import { PageWrapper } from '@/components/common/PageWrapper';

export function Home() {
    return (
        <PageWrapper className="h-full min-h-0 space-y-6 overflow-y-auto overscroll-contain rounded-t-3xl pb-24 scrollbar-none md:pb-4">
            <Total />
            <Activity />
            <StatsChart />
            <ChannelRanking />
        </PageWrapper>
    );
}
