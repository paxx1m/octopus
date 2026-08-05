import { Activity } from './activity';
import { Total } from './total';
import { StatsChart } from './chart';
import { Rank } from './rank';
import { ChannelPerf } from './channel-perf';
import { PageWrapper } from '@/components/common/PageWrapper';

export function Home() {
    return (
        <PageWrapper className="h-full min-h-0 space-y-6 overflow-y-auto overscroll-contain rounded-t-3xl pb-24 scrollbar-none md:pb-4">
            <Total />
            <ChannelPerf />
            <Activity />
            <StatsChart />
            <Rank />
        </PageWrapper>
    );
}
