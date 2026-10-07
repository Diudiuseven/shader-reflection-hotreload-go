package shader

import (
	"errors"
	"strings"
	"testing"
)

const baseSource = `#stage vertex
#stage fragment
in vec3 position @0
out vec4 color @0
cbuffer Camera @0 {
	mat4 viewProj;
}
uniform texture2D albedo @1
`

// 接口不变、实现变化（注释不同）
const implOnlyChange = `#stage vertex
#stage fragment
// 新的实现细节
in vec3 position @0
out vec4 color @0
cbuffer Camera @0 {
	mat4 viewProj;
}
uniform texture2D albedo @1
`

func mustModule(t *testing.T, src string) *ShaderModule {
	t.Helper()
	m, err := NewShaderModule([]byte(src))
	if err != nil {
		t.Fatalf("NewShaderModule: %v", err)
	}
	return m
}

func TestReloadInterfaceUnchangedPreservesState(t *testing.T) {
	r := NewReloader(mustModule(t, baseSource), PolicyUseOld)
	old := r.Current()
	if err := old.SetConstant("Camera", []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	if err := old.SetResource("albedo", 42); err != nil {
		t.Fatal(err)
	}

	if err := r.Reload([]byte(implOnlyChange)); err != nil {
		t.Fatalf("reload: %v", err)
	}
	cur := r.Current()
	if cur == old {
		t.Fatal("expected a new module version after reload")
	}
	got, ok := cur.Constant("Camera")
	if !ok || string(got[:4]) != string([]byte{1, 2, 3, 4}) {
		t.Errorf("constant lost after reload: %v %v", got, ok)
	}
	if h, ok := cur.Resource("albedo"); !ok || h != 42 {
		t.Errorf("resource binding lost after reload: %v %v", h, ok)
	}
}

func TestReloadIncompatible(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"binding changed", strings.Replace(baseSource, "albedo @1", "albedo @2", 1), "binding changed"},
		{"type changed", strings.Replace(baseSource, "in vec3 position", "in vec2 position", 1), "type changed"},
		{"stage changed", strings.Replace(baseSource, "#stage fragment\n", "", 1), "stages changed"},
		{"cbuffer removed", strings.Replace(baseSource, "cbuffer Camera @0 {\n\tmat4 viewProj;\n}\n", "", 1), "removed"},
		{"resource stages changed", strings.Replace(baseSource, "albedo @1", "albedo @1 [vertex]", 1), "stages changed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewReloader(mustModule(t, baseSource), PolicyUseOld)
			old := r.Current()
			err := r.Reload([]byte(tc.src))
			if err == nil {
				t.Fatal("expected incompatibility error, got nil")
			}
			var incErr *IncompatibleError
			if !errors.As(err, &incErr) {
				t.Fatalf("error type = %T, want *IncompatibleError (%v)", err, err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
			if r.Current() != old {
				t.Error("incompatible reload must not replace current version")
			}
		})
	}
}

func TestReloadCompileFailureKeepsOld(t *testing.T) {
	r := NewReloader(mustModule(t, baseSource), PolicyUseOld)
	old := r.Current()
	truncated := `#stage vertex
cbuffer Camera @0 {
	mat4 viewProj;
`
	err := r.Reload([]byte(truncated))
	if err == nil {
		t.Fatal("expected compile error, got nil")
	}
	var incErr *IncompatibleError
	if errors.As(err, &incErr) {
		t.Fatal("compile failure must not be reported as IncompatibleError")
	}
	if r.Current() != old {
		t.Error("failed reload must not replace current version")
	}
}

func TestReloadIdenticalSourceIsNoop(t *testing.T) {
	r := NewReloader(mustModule(t, baseSource), PolicyUseOld)
	old := r.Current()
	if err := r.Reload([]byte(baseSource)); err != nil {
		t.Fatal(err)
	}
	if r.Current() != old {
		t.Error("identical source should be a no-op")
	}
}

func TestReloadRevertAfterChange(t *testing.T) {
	r := NewReloader(mustModule(t, baseSource), PolicyUseOld)
	v0 := r.Current()
	// 改动 -> 回退，回退后应与 v0 摘要一致
	if err := r.Reload([]byte(implOnlyChange)); err != nil {
		t.Fatal(err)
	}
	v1 := r.Current()
	if v1 == v0 {
		t.Fatal("expected new version after change")
	}
	if err := r.Reload([]byte(baseSource)); err != nil {
		t.Fatal(err)
	}
	if r.Current().SourceSum() != v0.SourceSum() {
		t.Error("revert should restore original source sum")
	}
}

func TestReloadSequentialChanges(t *testing.T) {
	r := NewReloader(mustModule(t, baseSource), PolicyUseOld)
	// 连续多次改动：部分失败、部分成功，最终生效的必须是最后一个成功版本
	_ = r.Reload([]byte("garbage")) // 失败
	if err := r.Reload([]byte(implOnlyChange)); err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(baseSource, "albedo @1", "albedo @9", 1)
	if err := r.Reload([]byte(bad)); err == nil {
		t.Fatal("expected incompatibility")
	}
	final := r.Current()
	if got := final.Interface().Resources[0].Binding; got != 1 {
		t.Errorf("binding = %d, want 1 (last successful version)", got)
	}
}
