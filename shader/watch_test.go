package shader

import (
	"sync"
	"testing"
	"time"
)

// sourceStore 是并发安全的着色器源存储，模拟磁盘文件。
type sourceStore struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *sourceStore) set(path, src string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[path] = src
}

func (s *sourceStore) get(path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[path], nil
}

func TestWatcherDebounceAndOrder(t *testing.T) {
	p, r := newTestPipeline(t, baseSource)
	store := &sourceStore{m: map[string]string{"a.shader": baseSource}}
	w := NewWatcher(r, 20*time.Millisecond, store.get)
	defer w.Close()

	// 去抖窗口内连续 5 次改动，只应触发一次重载。
	for i := 0; i < 5; i++ {
		store.set("a.shader", baseSource+"// edit\n")
		w.Notify("a.shader")
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	evs := w.Events()
	if len(evs) != 1 {
		t.Fatalf("expected 1 debounced reload, got %d", len(evs))
	}
	if evs[0].Result != ReloadSwapped || evs[0].Err != nil {
		t.Fatalf("unexpected event: %+v", evs[0])
	}

	// 改成不兼容 -> 再改回兼容，顺序与结果可复现。
	store.set("a.shader", "#stage vertex\nin float3 pos @location(0)\n")
	w.Notify("a.shader")
	time.Sleep(80 * time.Millisecond)
	store.set("a.shader", baseSource)
	w.Notify("a.shader")
	time.Sleep(80 * time.Millisecond)

	evs = w.Events()
	if len(evs) != 3 {
		t.Fatalf("expected 3 events, got %d", len(evs))
	}
	if evs[1].Result != ReloadIncompatible {
		t.Fatalf("event 1: expected incompatible, got %+v", evs[1])
	}
	if evs[2].Result != ReloadSwapped {
		t.Fatalf("event 2: expected swapped, got %+v", evs[2])
	}
	// 最终管线接口回到 baseSource。
	if got := p.BeginFrame().Interface(); len(got.Resources) != 2 {
		t.Fatalf("pipeline not reverted: %+v", got)
	}
}

func TestWatcherCloseLeavesNoPending(t *testing.T) {
	_, r := newTestPipeline(t, baseSource)
	w := NewWatcher(r, time.Hour, func(string) (string, error) { return baseSource, nil })
	w.Notify("x.shader")
	w.Close()
	time.Sleep(20 * time.Millisecond)
	if n := len(w.Events()); n != 0 {
		t.Fatalf("closed watcher fired %d events", n)
	}
	// Close 后再 Notify 不得触发。
	w.Notify("x.shader")
	time.Sleep(20 * time.Millisecond)
	if n := len(w.Events()); n != 0 {
		t.Fatalf("notify after close fired %d events", n)
	}
}
