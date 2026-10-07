package shader

import (
	"sync"
	"time"
)

// Watcher 监听着色器来源变更，去抖后触发重载。
// 去抖窗口内的重复改动只触发一次重载；重载在单个 goroutine 中串行执行，
// 因此重载结果与触发顺序可复现。
type Watcher struct {
	readSource func() ([]byte, error)
	reloader   *Reloader
	debounce   time.Duration

	notifyCh chan struct{}
	done     chan struct{}
	wg       sync.WaitGroup

	mu      sync.Mutex
	lastErr error
	reloads int // 已执行的重载次数（含失败尝试）
}

// NewWatcher 创建监听器：变更时通过 readSource 读取源码并交给 reloader 重载。
// debounce 为去抖窗口；窗口内的重复 Notify 只触发一次重载。
func NewWatcher(readSource func() ([]byte, error), reloader *Reloader, debounce time.Duration) *Watcher {
	w := &Watcher{
		readSource: readSource,
		reloader:   reloader,
		debounce:   debounce,
		notifyCh:   make(chan struct{}, 1),
		done:       make(chan struct{}),
	}
	w.wg.Add(1)
	go w.loop()
	return w
}

// Notify 通知来源发生变更（可重复调用，会被去抖）。非阻塞。
func (w *Watcher) Notify() {
	select {
	case w.notifyCh <- struct{}{}:
	default: // 已有待处理通知，合并
	}
}

// LastError 返回最近一次重载的错误（无错误为 nil）。
func (w *Watcher) LastError() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastErr
}

// ReloadCount 返回已执行的重载次数。
func (w *Watcher) ReloadCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.reloads
}

func (w *Watcher) loop() {
	defer w.wg.Done()
	var timer *time.Timer
	var timerC <-chan time.Time
	pending := false
	for {
		select {
		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			return
		case <-w.notifyCh:
			pending = true
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(w.debounce)
			timerC = timer.C
		case <-timerC:
			if pending {
				pending = false
				w.reloadOnce()
			}
			timerC = nil
		}
	}
}

func (w *Watcher) reloadOnce() {
	src, err := w.readSource()
	if err == nil {
		err = w.reloader.Reload(src)
	}
	w.mu.Lock()
	w.lastErr = err
	w.reloads++
	w.mu.Unlock()
}

// Close 停止监听并等待后台 goroutine 退出，保证无残留临时状态。
func (w *Watcher) Close() error {
	close(w.done)
	w.wg.Wait()
	return nil
}
