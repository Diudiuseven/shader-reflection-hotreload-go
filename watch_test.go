package shader

import (
	"sync/atomic"
	"testing"
	"time"
)

// fakeSource 模拟可写着色器来源。
type fakeSource struct {
	data atomic.Value // []byte
}

func (s *fakeSource) read() ([]byte, error) {
	v := s.data.Load().([]byte)
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (s *fakeSource) write(src string) { s.data.Store([]byte(src)) }

func newFakeSource(init string) *fakeSource {
	s := &fakeSource{}
	s.write(init)
	return s
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", msg)
}

func TestWatcherDebounce(t *testing.T) {
	src := newFakeSource(baseSource)
	r := NewReloader(mustModule(t, baseSource), PolicyUseOld)
	w := NewWatcher(src.read, r, 30*time.Millisecond)
	defer w.Close()

	src.write(implOnlyChange)
	// 去抖窗口内重复通知
	for i := 0; i < 5; i++ {
		w.Notify()
		time.Sleep(5 * time.Millisecond)
	}
	waitFor(t, func() bool { return w.ReloadCount() > 0 }, "first reload")
	if n := w.ReloadCount(); n != 1 {
		t.Fatalf("reload count = %d, want 1 (debounced)", n)
	}
	if r.Current().SourceSum() != mustSum(t, implOnlyChange) {
		t.Error("watcher did not apply the change")
	}
}

func TestWatcherRapidSequentialChanges(t *testing.T) {
	src := newFakeSource(baseSource)
	r := NewReloader(mustModule(t, baseSource), PolicyUseOld)
	w := NewWatcher(src.read, r, 20*time.Millisecond)
	defer w.Close()

	// 快速连续改动 A->B->A：最终内容等于 A，重载应为空操作或收敛到 A
	src.write(implOnlyChange)
	w.Notify()
	src.write(baseSource)
	w.Notify()
	waitFor(t, func() bool { return w.ReloadCount() > 0 }, "reload")
	time.Sleep(50 * time.Millisecond)
	if r.Current().SourceSum() != mustSum(t, baseSource) {
		t.Error("final version should converge to latest source content")
	}
}

func TestWatcherFailureKeepsOldAndRecovers(t *testing.T) {
	src := newFakeSource(baseSource)
	r := NewReloader(mustModule(t, baseSource), PolicyUseOld)
	w := NewWatcher(src.read, r, 20*time.Millisecond)
	defer w.Close()
	old := r.Current()

	// 写入被截断的着色器：重载失败，旧版本保留
	src.write("#stage vertex\ncbuffer Broken @0 {\n")
	w.Notify()
	waitFor(t, func() bool { return w.LastError() != nil }, "reload error")
	if r.Current() != old {
		t.Error("failed reload must keep old version")
	}

	// 修复来源后恢复
	src.write(implOnlyChange)
	w.Notify()
	waitFor(t, func() bool {
		return r.Current().SourceSum() == mustSum(t, implOnlyChange)
	}, "recovery after fix")
}

func TestWatcherCloseNoLeak(t *testing.T) {
	src := newFakeSource(baseSource)
	r := NewReloader(mustModule(t, baseSource), PolicyUseOld)
	w := NewWatcher(src.read, r, 10*time.Millisecond)
	w.Notify()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	// Close 后 Notify 不应 panic 或阻塞
	w.Notify()
}
