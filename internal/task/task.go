package task

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/bestruirui/octopus/internal/utils/log"
)

type taskEntry struct {
	name       string
	interval   time.Duration
	fn         func()
	runOnStart bool
	ticker     *time.Ticker
	stopCh     chan struct{}
	updateCh   chan time.Duration
	running    atomic.Bool
}

var (
	tasks   = make(map[string]*taskEntry)
	tasksMu sync.RWMutex
	stopped atomic.Bool
)

// Register 注册一个定时任务
// runOnStart: 是否在启动时立即执行一次
func Register(name string, interval time.Duration, runOnStart bool, fn func()) {
	if interval <= 0 {
		log.Debugf("task %s not registered: interval is 0", name)
		return
	}

	tasksMu.Lock()
	defer tasksMu.Unlock()

	if _, exists := tasks[name]; exists {
		log.Warnf("task %s already registered, skipping", name)
		return
	}

	tasks[name] = &taskEntry{
		name:       name,
		interval:   interval,
		fn:         fn,
		runOnStart: runOnStart,
		stopCh:     make(chan struct{}),
		updateCh:   make(chan time.Duration, 1),
	}
	log.Debugf("task %s registered with interval %v, runOnStart: %v", name, interval, runOnStart)
}

// Update 更新任务的执行间隔
// 当 interval 为 0 时，删除任务
func Update(name string, interval time.Duration) {
	tasksMu.Lock()
	entry, exists := tasks[name]
	if !exists {
		tasksMu.Unlock()
		log.Warnf("task %s not found", name)
		return
	}

	if interval <= 0 {
		delete(tasks, name)
		tasksMu.Unlock()
		select {
		case <-entry.stopCh:
		default:
			close(entry.stopCh)
		}
		log.Infof("task %s removed: interval is 0", name)
		return
	}
	tasksMu.Unlock()

	select {
	case entry.updateCh <- interval:
		log.Infof("task %s interval updated to %v", name, interval)
	default:
		log.Warnf("task %s update channel full, skipping", name)
	}
}

// RUN 启动所有注册的任务（非阻塞，各任务在独立 goroutine 中运行）。
func RUN() {
	tasksMu.RLock()
	for _, entry := range tasks {
		go runTask(entry)
	}
	tasksMu.RUnlock()
}

// Stop 停止全部后台任务，并等待当前 in-flight 执行结束（最长 wait）。
func Stop(wait time.Duration) {
	if !stopped.CompareAndSwap(false, true) {
		return
	}
	tasksMu.Lock()
	entries := make([]*taskEntry, 0, len(tasks))
	for _, e := range tasks {
		entries = append(entries, e)
		select {
		case <-e.stopCh:
		default:
			close(e.stopCh)
		}
	}
	tasks = make(map[string]*taskEntry)
	tasksMu.Unlock()

	deadline := time.Now().Add(wait)
	for _, e := range entries {
		for e.running.Load() && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
	}

	// 等待手动触发（/channel/sync）的同步 goroutine 结束，避免其继续写已关闭的 DB
	manualSyncDone := make(chan struct{})
	go func() {
		manualSyncWG.Wait()
		close(manualSyncDone)
	}()
	select {
	case <-manualSyncDone:
	case <-time.After(wait):
		log.Warnf("manual model sync did not finish within %v", wait)
	}
	log.Infof("background tasks stopped")
}

func runTask(entry *taskEntry) {
	safeRun := func() {
		if !entry.running.CompareAndSwap(false, true) {
			log.Debugf("task %s skipped: previous run still in progress", entry.name)
			return
		}
		defer entry.running.Store(false)
		if stopped.Load() {
			return
		}
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("task %s panicked: %v", entry.name, r)
			}
		}()
		entry.fn()
	}

	if entry.runOnStart {
		go safeRun()
	}

	entry.ticker = time.NewTicker(entry.interval)
	defer entry.ticker.Stop()

	for {
		select {
		case <-entry.ticker.C:
			go safeRun()
		case newInterval := <-entry.updateCh:
			entry.ticker.Stop()
			entry.interval = newInterval
			entry.ticker = time.NewTicker(newInterval)
		case <-entry.stopCh:
			return
		}
	}
}
