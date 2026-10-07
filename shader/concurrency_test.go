package shader

import (
	"sync"
	"testing"
)

// 重载与渲染提交并发：任意帧快照必须是某一个完整版本，
// 不得出现新旧混合（此处用接口指针与绑定状态的一致性验证）。
func TestReloadConcurrentWithFrames(t *testing.T) {
	p, r := newTestPipeline(t, baseSource)
	p.SetConstant("MVP", []byte{0xAA})
	p.BindResource("albedo", 7)

	const frames = 200
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 提交方：不断取帧快照并校验一致性。
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < frames; i++ {
				select {
				case <-stop:
					return
				default:
				}
				f := p.BeginFrame()
				iface := f.Interface()
				if len(iface.ConstantBuffers) != 1 || iface.ConstantBuffers[0].Name != "MVP" {
					t.Errorf("frame saw torn interface: %+v", iface)
					return
				}
				// 帧快照期间重载不得影响已取得的快照。
				if got := f.Interface(); got != iface {
					t.Error("frame snapshot mutated underneath reader")
					return
				}
			}
		}()
	}

	// 重载方：交替做兼容重载与失败重载。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if _, err := r.Reload(baseSource + "// v\n"); err != nil {
				t.Errorf("compatible reload failed: %v", err)
				return
			}
			if res, _ := r.Reload("#stage vertex\nin float3"); res != ReloadCompileFailed {
				t.Errorf("expected compile failure, got %v", res)
				return
			}
		}
	}()

	wg.Wait()
	close(stop)
	// 重载结束后旧状态仍在。
	if v, ok := p.Constant("MVP"); !ok || v[0] != 0xAA {
		t.Fatal("constant lost under concurrent reload")
	}
	if h, ok := p.Resource("albedo"); !ok || h != 7 {
		t.Fatal("resource lost under concurrent reload")
	}
}
