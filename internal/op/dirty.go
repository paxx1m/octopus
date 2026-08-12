package op

import "sync"

// dirtySet 并发安全的脏标记集合：Mark 打标，Snapshot 摘出并清空，
// 落库失败时 Remerge 回填，避免运行时状态（费用/状态码）丢失。
// 统一了 stats 三组与 channel key 一组原本重复的「map + mutex + snapshot/remerge」样板。
type dirtySet struct {
	mu  sync.Mutex
	ids map[int]struct{}
}

func newDirtySet() *dirtySet {
	return &dirtySet{ids: make(map[int]struct{})}
}

func (d *dirtySet) Mark(id int) {
	d.mu.Lock()
	d.ids[id] = struct{}{}
	d.mu.Unlock()
}

// IsDirty 返回 id 是否仍处于待落库状态。
func (d *dirtySet) IsDirty(id int) bool {
	d.mu.Lock()
	_, ok := d.ids[id]
	d.mu.Unlock()
	return ok
}

// Snapshot 摘出全部脏 id 并清空集合。
func (d *dirtySet) Snapshot() []int {
	d.mu.Lock()
	defer d.mu.Unlock()
	ids := make([]int, 0, len(d.ids))
	for id := range d.ids {
		ids = append(ids, id)
	}
	d.ids = make(map[int]struct{})
	return ids
}

// SnapshotFiltered 摘出满足 pred 的脏 id（其余保留）。
func (d *dirtySet) SnapshotFiltered(pred func(id int) bool) []int {
	d.mu.Lock()
	defer d.mu.Unlock()
	ids := make([]int, 0)
	for id := range d.ids {
		if pred(id) {
			ids = append(ids, id)
			delete(d.ids, id)
		}
	}
	return ids
}

// Remerge 将落库失败的 id 回填为脏。
func (d *dirtySet) Remerge(ids []int) {
	if len(ids) == 0 {
		return
	}
	d.mu.Lock()
	for _, id := range ids {
		d.ids[id] = struct{}{}
	}
	d.mu.Unlock()
}

func (d *dirtySet) Remove(ids ...int) {
	d.mu.Lock()
	for _, id := range ids {
		delete(d.ids, id)
	}
	d.mu.Unlock()
}

func (d *dirtySet) Clear() {
	d.mu.Lock()
	d.ids = make(map[int]struct{})
	d.mu.Unlock()
}
