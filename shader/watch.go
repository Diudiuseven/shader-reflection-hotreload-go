package shader

import (
	"sync"
	"time"
)

// WatchEvent 是一次去抖后的重载触发。
type WatchEvent struct {
	Path   string
	Result ReloadResult
	Err    error
}

// Watcher 监听着色器来源变更，去抖后触发重载。
// 重复改动只触发一次重载；触发顺序与结果可复现。
type Watcher struct {
	mu       sync.Mutex
	reloader *Reloader
	debounce time.Duration
	read     func(path string) (string, error) // 读取源文本
	pending  map[string]*time.Timer
	events   []WatchEvent
	closed   bool
}

// NewWatcher 构造监听器。read 用于在触发时读取最新源文本。
func NewWatcher(r *Reloader, debounce time.Duration, read func(string) (string, error)) *Watcher {
	return &Watcher{
		reloader: r,
		debounce: debounce,
		read:     read,
		pending:  make(map[string]*time.Timer),
	}
}

// Notify 通知某路径发生变化；在去抖窗口内的重复通知会被合并。
func (w *Watcher) Notify(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if t, ok := w.pending[path]; ok {
		t.Stop()
	}
	w.pending[path] = time.AfterFunc(w.debounce, func() { w.fire(path) })
}

func (w *Watcher) fire(path string) {
	src, err := w.read(path)
	var res ReloadResult
	if err != nil {
		res = ReloadCompileFailed
	} else {
		res, err = w.reloader.Reload(src)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.pending, path)
	w.events = append(w.events, WatchEvent{Path: path, Result: res, Err: err})
}

// Events 返回已发生的重载事件（按触发顺序）。
func (w *Watcher) Events() []WatchEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]WatchEvent, len(w.events))
	copy(out, w.events)
	return out
}

// Close 停止监听器并清理所有挂起的定时器，不残留临时状态。
func (w *Watcher) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	for _, t := range w.pending {
		t.Stop()
	}
	w.pending = make(map[string]*time.Timer)
}
