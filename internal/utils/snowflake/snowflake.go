package snowflake

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"
)

var (
	sfMutex    sync.Mutex
	sfLastTime int64
	sfSeq      int64
	sfWorker   int64
)

func init() {
	var b [2]byte
	if _, err := rand.Read(b[:]); err == nil {
		sfWorker = int64(binary.BigEndian.Uint16(b[:])) & 0x3FF // 10-bit worker id
	}
}

// GenerateID 生成唯一ID（简化 snowflake：41bit 时间 + 10bit worker + 12bit 序列）
func GenerateID() int64 {
	sfMutex.Lock()
	defer sfMutex.Unlock()

	now := time.Now().UnixMilli()
	if now == sfLastTime {
		sfSeq = (sfSeq + 1) & 0xFFF
		if sfSeq == 0 {
			for now <= sfLastTime {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		sfSeq = 0
	}
	sfLastTime = now

	// 41 bits time | 10 bits worker | 12 bits sequence
	return (now << 22) | (sfWorker << 12) | sfSeq
}
