package shader

import "testing"

const baseSource = `#stage vertex
#stage fragment
in float3 pos @location(0)
in float2 uv @location(1)
out float4 color @location(0)
const MVP @binding(0) size(64) stages(vertex)
texture2d albedo @binding(1) stages(fragment)
sampler samp @binding(2) stages(fragment)
`

func TestReflectDeterministic(t *testing.T) {
	a, err := ReflectSource(baseSource)
	if err != nil {
		t.Fatalf("reflect: %v", err)
	}
	b, err := ReflectSource(baseSource)
	if err != nil {
		t.Fatalf("reflect: %v", err)
	}
	if err := CheckCompatible(a, b); err != nil {
		t.Fatalf("identical sources must be compatible: %v", err)
	}
	if len(a.Stages) != 2 || len(a.Inputs) != 2 || len(a.Outputs) != 1 ||
		len(a.ConstantBuffers) != 1 || len(a.Resources) != 2 {
		t.Fatalf("unexpected interface: %+v", a)
	}
	if a.Inputs[0].Name != "pos" || a.Inputs[0].Location != 0 || a.Inputs[0].Type != TypeFloat3 {
		t.Fatalf("bad input reflection: %+v", a.Inputs[0])
	}
	if a.ConstantBuffers[0].Binding != 0 || a.ConstantBuffers[0].Size != 64 ||
		!a.ConstantBuffers[0].Stages.Has(StageVertex) {
		t.Fatalf("bad constant buffer: %+v", a.ConstantBuffers[0])
	}
}

func TestReflectTruncatedSource(t *testing.T) {
	truncated := "#stage vertex\nin float3 pos @loca"
	_, err := ReflectSource(truncated)
	if err == nil {
		t.Fatal("expected error for truncated source")
	}
	if !IsCompileError(err) {
		t.Fatalf("expected CompileError, got %T: %v", err, err)
	}
	ce := err.(*CompileError)
	if ce.Line != 2 {
		t.Fatalf("expected line 2, got %d", ce.Line)
	}
}

func TestReflectEmptySource(t *testing.T) {
	if _, err := ReflectSource(""); !IsCompileError(err) {
		t.Fatalf("expected CompileError for empty source, got %v", err)
	}
}

func TestReflectCompiledMatchesSource(t *testing.T) {
	c := NewFakeCompiler()
	m, err := c.Compile("base", baseSource)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	fromSrc, _ := ReflectSource(baseSource)
	fromBin, err := ReflectCompiled(m)
	if err != nil {
		t.Fatalf("reflect compiled: %v", err)
	}
	if err := CheckCompatible(fromSrc, fromBin); err != nil {
		t.Fatalf("source and compiled reflection must agree: %v", err)
	}
}
