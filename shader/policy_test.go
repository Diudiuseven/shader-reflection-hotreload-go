package shader

import (
	"testing"
	"time"
)

// blockingCompiler 在编译时阻塞，直到测试放行，用于确定性地制造“重载进行中”。
type blockingCompiler struct {
	inner   Compiler
	entered chan struct{}
	release chan struct{}
}

func (c *blockingCompiler) Compile(name, source string) (*CompiledModule, error) {
	close(c.entered)
	<-c.release
	return c.inner.Compile(name, source)
}

func newBlockingReloader(t *testing.T, policy ReloadPolicy) (*Pipeline, *Reloader, *blockingCompiler) {
	t.Helper()
	fake := NewFakeCompiler()
	m, err := fake.Compile("test", baseSource)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	p, err := NewPipeline(m)
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	bc := &blockingCompiler{inner: fake, entered: make(chan struct{}), release: make(chan struct{})}
	return p, NewReloader(p, bc, policy), bc
}

func TestPolicyUseOldDuringReload(t *testing.T) {
	p, r, bc := newBlockingReloader(t, PolicyUseOld)
	done := make(chan ReloadResult, 1)
	go func() {
		res, _ := r.Reload(baseSource + "// new impl\n")
		done <- res
	}()
	<-bc.entered // 重载临界区已开始

	// 重载期间新到的帧必须立即拿到旧版本快照，不阻塞、不读到半更新状态。
	select {
	case <-time.After(2 * time.Second):
		t.Fatal("BeginFrame blocked under PolicyUseOld")
	default:
	}
	f := p.BeginFrame()
	if len(f.Interface().Resources) != 2 {
		t.Fatalf("stale frame incomplete: %+v", f.Interface())
	}

	close(bc.release)
	if res := <-done; res != ReloadSwapped {
		t.Fatalf("reload result: %v", res)
	}
}

func TestPolicyWaitDuringReload(t *testing.T) {
	p, r, bc := newBlockingReloader(t, PolicyWait)
	done := make(chan ReloadResult, 1)
	go func() {
		res, _ := r.Reload(baseSource + "// new impl\n")
		done <- res
	}()
	<-bc.entered

	frameDone := make(chan struct{})
	go func() {
		p.BeginFrame()
		close(frameDone)
	}()
	// 重载未放行时，等待策略的帧必须阻塞。
	select {
	case <-frameDone:
		t.Fatal("BeginFrame should block under PolicyWait during reload")
	case <-time.After(50 * time.Millisecond):
	}
	close(bc.release)
	<-frameDone
	if res := <-done; res != ReloadSwapped {
		t.Fatalf("reload result: %v", res)
	}
}
