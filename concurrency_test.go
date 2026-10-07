package shader

import (
	"sync"
	"testing"
	"time"
)

// 交替出现的两个接口兼容版本
var versionA = baseSource
var versionB = implOnlyChange

// TestConcurrentReloadAndFrames 重载与帧提交并发进行：
// 任何一帧读到的版本必须是完整的 vA 或 vB，不得出现混合状态。
func TestConcurrentReloadAndFrames(t *testing.T) {
	for _, policy := range []ReloadPolicy{PolicyUseOld, PolicyWait} {
		t.Run(map[ReloadPolicy]string{PolicyUseOld: "UseOld", PolicyWait: "Wait"}[policy], func(t *testing.T) {
			r := NewReloader(mustModule(t, versionA), policy)
			sumA := r.Current().SourceSum()

			var wg sync.WaitGroup
			stop := make(chan struct{})

			// 帧提交方
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-stop:
							return
						default:
						}
						f := r.BeginFrame()
						m := f.Module()
						sum := m.SourceSum()
						iface := m.Interface() // 读接口期间模拟帧使用
						if len(iface.Stages) != 2 {
							t.Errorf("frame saw incomplete interface: %+v", iface)
						}
						if sum != sumA && sum != mustSum(t, versionB) {
							t.Errorf("frame saw unknown version %x", sum)
						}
						f.End()
					}
				}()
			}

			// 重载方：连续多次改动（含回退）
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 50; i++ {
					src := versionB
					if i%2 == 1 {
						src = versionA
					}
					if err := r.Reload([]byte(src)); err != nil {
						t.Errorf("reload: %v", err)
						return
					}
				}
			}()

			time.Sleep(50 * time.Millisecond)
			close(stop)
			wg.Wait()
		})
	}
}

func mustSum(t *testing.T, src string) [16]byte {
	t.Helper()
	m, err := Compile([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return m.sourceSum
}

// TestFrameSnapshotStableDuringReload 帧存续期间重载发生，帧仍看到开始时的版本。
func TestFrameSnapshotStableDuringReload(t *testing.T) {
	r := NewReloader(mustModule(t, versionA), PolicyUseOld)
	f := r.BeginFrame()
	before := f.Module()
	if err := r.Reload([]byte(versionB)); err != nil {
		t.Fatal(err)
	}
	if f.Module() != before {
		t.Error("frame snapshot changed during reload")
	}
	f.End()
	if r.Current() == before {
		t.Error("Current should advance after reload")
	}
}

// TestPolicyWaitBlocksFramesDuringReload PolicyWait 下重载期间新帧等待，
// 等待结束后看到的是新版本。
func TestPolicyWaitBlocksFramesDuringReload(t *testing.T) {
	r := NewReloader(mustModule(t, versionA), PolicyWait)

	// 持有一个帧（模拟正在提交的帧），随后启动重载
	inFlight := r.BeginFrame()
	reloadDone := make(chan error, 1)
	go func() { reloadDone <- r.Reload([]byte(versionB)) }()

	// 等重载拿到帧写锁前的间隙，启动一个新帧；它应阻塞至重载完成
	time.Sleep(10 * time.Millisecond)
	frameDone := make(chan *ShaderModule, 1)
	go func() {
		f2 := r.BeginFrame()
		m := f2.Module()
		f2.End()
		frameDone <- m
	}()

	inFlight.End()
	if err := <-reloadDone; err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-frameDone:
		if m.SourceSum() != mustSum(t, versionB) {
			t.Error("frame started during reload should see the new version under PolicyWait")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("frame blocked forever under PolicyWait")
	}
}
