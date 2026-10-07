package shader

import (
	"reflect"
	"testing"
)

const sampleSource = `#stage vertex
#stage fragment
in vec3 position @0
in vec2 uv @1
out vec4 color @0
cbuffer Camera @0 {
	mat4 viewProj;
	vec3 eye;
}
uniform texture2D albedo @1 [fragment]
uniform sampler2D samp @2
`

func TestReflectFromSource(t *testing.T) {
	iface, err := ReflectFromSource([]byte(sampleSource))
	if err != nil {
		t.Fatalf("reflect: %v", err)
	}
	wantStages := []Stage{StageVertex, StageFragment}
	if !reflect.DeepEqual(iface.Stages, wantStages) {
		t.Errorf("stages = %v, want %v", iface.Stages, wantStages)
	}
	if len(iface.Inputs) != 2 || iface.Inputs[0].Name != "position" || iface.Inputs[0].Location != 0 {
		t.Errorf("inputs = %+v", iface.Inputs)
	}
	if iface.Inputs[1].Name != "uv" || iface.Inputs[1].Type != TypeVec2 || iface.Inputs[1].Location != 1 {
		t.Errorf("inputs[1] = %+v", iface.Inputs[1])
	}
	if len(iface.Outputs) != 1 || iface.Outputs[0].Name != "color" {
		t.Errorf("outputs = %+v", iface.Outputs)
	}
	if len(iface.ConstantBuffer) != 1 {
		t.Fatalf("cbuffers = %+v", iface.ConstantBuffer)
	}
	cb := iface.ConstantBuffer[0]
	if cb.Name != "Camera" || cb.Binding != 0 {
		t.Errorf("cbuffer = %+v", cb)
	}
	// mat4: offset 0 size 64; vec3: 对齐 16 -> offset 64 size 16；总大小 80
	wantFields := []ConstantField{
		{Name: "viewProj", Type: TypeMat4, Offset: 0, Size: 64},
		{Name: "eye", Type: TypeVec3, Offset: 64, Size: 16},
	}
	if !reflect.DeepEqual(cb.Fields, wantFields) {
		t.Errorf("fields = %+v, want %+v", cb.Fields, wantFields)
	}
	if cb.Size != 80 {
		t.Errorf("cb.Size = %d, want 80", cb.Size)
	}
	if len(iface.Resources) != 2 {
		t.Fatalf("resources = %+v", iface.Resources)
	}
	if iface.Resources[0].Name != "albedo" || iface.Resources[0].Binding != 1 ||
		!reflect.DeepEqual(iface.Resources[0].Stages, []Stage{StageFragment}) {
		t.Errorf("resources[0] = %+v", iface.Resources[0])
	}
	// 未显式声明阶段的资源默认覆盖全部阶段
	if !reflect.DeepEqual(iface.Resources[1].Stages, wantStages) {
		t.Errorf("resources[1].Stages = %v, want %v", iface.Resources[1].Stages, wantStages)
	}
}

func TestReflectDeterministic(t *testing.T) {
	a, err := ReflectFromSource([]byte(sampleSource))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		b, err := ReflectFromSource([]byte(sampleSource))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("reflection not repeatable:\n%+v\n%+v", a, b)
		}
	}
}

func TestReflectFromModuleMatchesSource(t *testing.T) {
	mod, err := Compile([]byte(sampleSource))
	if err != nil {
		t.Fatal(err)
	}
	fromMod, err := ReflectFromModule(mod)
	if err != nil {
		t.Fatal(err)
	}
	fromSrc, err := ReflectFromSource([]byte(sampleSource))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromMod, fromSrc) {
		t.Fatalf("module reflection != source reflection:\n%+v\n%+v", fromMod, fromSrc)
	}
}

func TestReflectTruncatedCBuffer(t *testing.T) {
	truncated := `#stage vertex
cbuffer Camera @0 {
	mat4 viewProj;
`
	if _, err := ReflectFromSource([]byte(truncated)); err == nil {
		t.Fatal("expected error for truncated cbuffer, got nil")
	}
	if _, err := Compile([]byte(truncated)); err == nil {
		t.Fatal("expected compile error for truncated source, got nil")
	}
}

func TestReflectBindingConflict(t *testing.T) {
	src := `#stage vertex
cbuffer A @0 {
	float x;
}
uniform texture2D tex @0
`
	if _, err := ReflectFromSource([]byte(src)); err == nil {
		t.Fatal("expected binding conflict error, got nil")
	}
}

func TestReflectNoStage(t *testing.T) {
	if _, err := ReflectFromSource([]byte("in vec3 p @0\n")); err == nil {
		t.Fatal("expected error for missing #stage, got nil")
	}
}
