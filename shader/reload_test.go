package shader

import (
	"errors"
	"testing"
)

func newTestPipeline(t *testing.T, src string) (*Pipeline, *Reloader) {
	t.Helper()
	c := NewFakeCompiler()
	m, err := c.Compile("test", src)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	p, err := NewPipeline(m)
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	return p, NewReloader(p, c, PolicyUseOld)
}

func TestReloadInterfaceUnchangedPreservesState(t *testing.T) {
	p, r := newTestPipeline(t, baseSource)
	if err := p.SetConstant("MVP", []byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("set constant: %v", err)
	}
	if err := p.BindResource("albedo", 42); err != nil {
		t.Fatalf("bind resource: %v", err)
	}
	// 只改实现细节（注释），接口不变。
	changed := baseSource + "// implementation tweaked\n"
	res, err := r.Reload(changed)
	if err != nil || res != ReloadSwapped {
		t.Fatalf("expected swap, got %v, %v", res, err)
	}
	if v, ok := p.Constant("MVP"); !ok || len(v) != 4 || v[0] != 1 {
		t.Fatalf("constant lost after reload: %v %v", v, ok)
	}
	if h, ok := p.Resource("albedo"); !ok || h != 42 {
		t.Fatalf("resource binding lost after reload: %v %v", h, ok)
	}
}

func TestReloadIncompatibleKeepsOld(t *testing.T) {
	p, r := newTestPipeline(t, baseSource)
	p.SetConstant("MVP", []byte{9})
	// 绑定位置变化 -> 不兼容。
	bad := `#stage vertex
#stage fragment
in float3 pos @location(0)
in float2 uv @location(1)
out float4 color @location(0)
const MVP @binding(5) size(64) stages(vertex)
texture2d albedo @binding(1) stages(fragment)
sampler samp @binding(2) stages(fragment)
`
	res, err := r.Reload(bad)
	if res != ReloadIncompatible {
		t.Fatalf("expected incompatible, got %v", res)
	}
	var ie *IncompatibleError
	if !errors.As(err, &ie) {
		t.Fatalf("expected IncompatibleError, got %T: %v", err, err)
	}
	found := false
	for _, reason := range ie.Reasons {
		if reason == ReasonBindingChanged {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ReasonBindingChanged in %v", ie.Reasons)
	}
	// 旧版本继续可用，状态未丢。
	if v, ok := p.Constant("MVP"); !ok || v[0] != 9 {
		t.Fatal("old version state lost after incompatible reload")
	}
	if len(p.BeginFrame().Interface().ConstantBuffers) != 1 ||
		p.BeginFrame().Interface().ConstantBuffers[0].Binding != 0 {
		t.Fatal("old interface not preserved")
	}
}

func TestReloadIncompatibleTypeAndStage(t *testing.T) {
	_, r := newTestPipeline(t, baseSource)
	bad := `#stage vertex
#stage fragment
in float3 pos @location(0)
in float2 uv @location(1)
out float4 color @location(0)
const MVP @binding(0) size(64) stages(vertex,fragment)
texture2d albedo @binding(1) stages(fragment)
sampler samp @binding(2) stages(fragment)
`
	res, err := r.Reload(bad)
	if res != ReloadIncompatible {
		t.Fatalf("expected incompatible, got %v", res)
	}
	var ie *IncompatibleError
	if !errors.As(err, &ie) {
		t.Fatalf("expected IncompatibleError, got %v", err)
	}
	hasStage := false
	for _, reason := range ie.Reasons {
		if reason == ReasonStageChanged {
			hasStage = true
		}
	}
	if !hasStage {
		t.Fatalf("expected ReasonStageChanged in %v", ie.Reasons)
	}
}

func TestReloadCompileFailureKeepsOld(t *testing.T) {
	p, r := newTestPipeline(t, baseSource)
	p.SetConstant("MVP", []byte{7})
	res, err := r.Reload("#stage vertex\nin float3 pos @loca") // 截断
	if res != ReloadCompileFailed {
		t.Fatalf("expected compile failure, got %v", res)
	}
	if !IsCompileError(err) {
		t.Fatalf("expected CompileError, got %v", err)
	}
	if v, ok := p.Constant("MVP"); !ok || v[0] != 7 {
		t.Fatal("state lost after failed reload")
	}
	// 失败后再次用合法源重载仍可成功。
	res, err = r.Reload(baseSource + "// ok\n")
	if res != ReloadSwapped || err != nil {
		t.Fatalf("reload after failure should work: %v %v", res, err)
	}
}

func TestReloadRepeatedChangesAndRevert(t *testing.T) {
	p, r := newTestPipeline(t, baseSource)
	p.SetConstant("MVP", []byte{1})
	// 连续多次接口不变的改动。
	for i := 0; i < 5; i++ {
		res, err := r.Reload(baseSource + "// rev\n")
		if res != ReloadSwapped || err != nil {
			t.Fatalf("reload %d: %v %v", i, res, err)
		}
	}
	// 改成不兼容接口。
	incompat := `#stage vertex
in float3 pos @location(0)
out float4 color @location(0)
`
	if res, err := r.Reload(incompat); res != ReloadIncompatible || err == nil {
		t.Fatalf("expected incompatible: %v %v", res, err)
	}
	// 回退到原接口：必须恢复可用，且状态保留。
	res, err := r.Reload(baseSource)
	if res != ReloadSwapped || err != nil {
		t.Fatalf("revert reload: %v %v", res, err)
	}
	if v, ok := p.Constant("MVP"); !ok || v[0] != 1 {
		t.Fatal("state lost after revert")
	}
}
