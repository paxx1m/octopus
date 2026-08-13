package op

import (
	"sync"

	"github.com/bestruirui/octopus/internal/utils/cache"
)

// statsShardCount 是分片化 stats 的桶数。
// 2 的幂配合位掩码取模，16 桶在典型部署规模下足以分散竞争。
const statsShardCount = 16

// shardedStats 按 ID 分桶的 stats 缓存 + 脏标记 + 读写锁。
// 替代原先「单个 RWMutex 守护全局 cache」的方案：
// 每次 relay 请求只锁目标分片，避免所有渠道的 stats 写入互相串行化。
type shardedStats[V any] struct {
	shards []*statsShard[V]
	mask   int
}

// statsShard 单个分片：cache 已自带分片，这里再叠加 dirty 与锁，
// 使 Get/Set/Update/Del 都只锁本分片。
type statsShard[V any] struct {
	mu    sync.RWMutex
	dirty *dirtySet
}

// newShardedStats 创建带 statsShardCount 分片的 shardedStats。
// cache 仍使用 1024 内部分片以降低读竞争，dirty 与锁按 16 桶粗粒度分片。
func newShardedStats[V any]() *shardedStats[V] {
	n := statsShardCount
	p := 1
	for p<<1 <= n {
		p <<= 1
	}
	s := &shardedStats[V]{
		shards: make([]*statsShard[V], p),
		mask:   p - 1,
	}
	for i := 0; i < p; i++ {
		s.shards[i] = &statsShard[V]{dirty: newDirtySet()}
	}
	return s
}

func (s *shardedStats[V]) shard(id int) *statsShard[V] {
	return s.shards[id&s.mask]
}

// getOrCreate 读缓存，未命中则写入零值占位（不标记 dirty）。
// 回写零值占位避免后续请求重复穿透；待 Update 真正累加时才 Mark dirty。
func (s *shardedStats[V]) getOrCreate(id int, c cache.Cache[int, V], zero V) (V, bool) {
	sh := s.shard(id)
	// 快速路径：读锁命中直接返回。
	sh.mu.RLock()
	v, ok := c.Get(id)
	sh.mu.RUnlock()
	if ok {
		return v, true
	}

	// 慢路径：写锁 double-check 后写入零值占位。
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if v, ok := c.Get(id); ok {
		return v, true
	}
	c.Set(id, zero)
	return zero, false
}

// update 在目标分片内读取、累加并回写（通过 mutate 回调），同时标记 dirty。
func (s *shardedStats[V]) update(id int, c cache.Cache[int, V], zero V, mutate func(v *V)) {
	sh := s.shard(id)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	v, ok := c.Get(id)
	if !ok {
		v = zero
	}
	mutate(&v)
	c.Set(id, v)
	sh.dirty.Mark(id)
}

// remove 从目标分片删除缓存与脏标记。
func (s *shardedStats[V]) remove(id int, c cache.Cache[int, V]) {
	sh := s.shard(id)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	c.Del(id)
	sh.dirty.Remove(id)
}

// removeAndDeleteDB 在目标分片删除缓存与脏标记，并执行 DB 删除。
// exists 为 true 时执行 dbDelete，避免对不存在的行发无意义 SQL。
func (s *shardedStats[V]) removeAndDeleteDB(id int, c cache.Cache[int, V], dbDelete func(id int) error) error {
	sh := s.shard(id)
	sh.mu.Lock()
	_, ok := c.Get(id)
	if ok {
		c.Del(id)
		sh.dirty.Remove(id)
	}
	sh.mu.Unlock()
	if !ok {
		return nil
	}
	return dbDelete(id)
}

// snapshotAll 摘出所有分片的脏 id 并清空，返回各分片快照（按 shard 顺序）。
// 落库失败时用 remergeTracked 回填，保证运行时累计状态不丢失。
func (s *shardedStats[V]) snapshotAll() [][]int {
	tracked := make([][]int, len(s.shards))
	for i, sh := range s.shards {
		tracked[i] = sh.dirty.Snapshot()
	}
	return tracked
}

// remergeTracked 将落库失败的 id 按分片回填为脏。
func (s *shardedStats[V]) remergeTracked(tracked [][]int) {
	for i, ids := range tracked {
		if i >= len(s.shards) {
			break
		}
		s.shards[i].dirty.Remerge(ids)
	}
}

// clearAll 清空所有分片的缓存与脏标记（配合外部 cache.Clear）。
func (s *shardedStats[V]) clearAll(c cache.Cache[int, V]) {
	for _, sh := range s.shards {
		sh.mu.Lock()
		sh.dirty.Clear()
		sh.mu.Unlock()
	}
	c.Clear()
}

// loadInto 将 DB 加载的数据写入指定分片（不标记 dirty）。
func (s *shardedStats[V]) loadInto(id int, c cache.Cache[int, V], v V) {
	sh := s.shard(id)
	sh.mu.Lock()
	c.Set(id, v)
	sh.mu.Unlock()
}

// flattenInts 将分片化的脏 id 二维切片展平为一维，供 persistStatsSnapshots 遍历。
func flattenInts(sharded [][]int) []int {
	total := 0
	for _, s := range sharded {
		total += len(s)
	}
	if total == 0 {
		return nil
	}
	out := make([]int, 0, total)
	for _, s := range sharded {
		out = append(out, s...)
	}
	return out
}
