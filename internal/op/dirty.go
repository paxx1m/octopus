package op

import "sync"

func snapshotDirtyIDs(m *map[int]struct{}, lock *sync.Mutex) []int {
	lock.Lock()
	ids := make([]int, 0, len(*m))
	for id := range *m {
		ids = append(ids, id)
	}
	*m = make(map[int]struct{})
	lock.Unlock()
	return ids
}

func remergeDirtyIDs(m *map[int]struct{}, lock *sync.Mutex, ids []int) {
	if len(ids) == 0 {
		return
	}
	lock.Lock()
	for _, id := range ids {
		(*m)[id] = struct{}{}
	}
	lock.Unlock()
}
