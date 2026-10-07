package shader

import (
	"fmt"
	"strings"
)

// IncompatibleError 表示新旧接口不兼容，Reasons 列出全部不兼容点。
// 调用方可用 errors.As 将其与编译错误等其它失败区分开。
type IncompatibleError struct {
	Reasons []string
}

func (e *IncompatibleError) Error() string {
	return "shader interface incompatible: " + strings.Join(e.Reasons, "; ")
}

// CheckCompatible 判定新接口是否与既有接口兼容。
// 绑定位置、类型或所需阶段发生变化均判定为不兼容。
// 兼容返回 nil，否则返回 *IncompatibleError（含全部不兼容原因）。
func CheckCompatible(oldIface, newIface *ShaderInterface) error {
	var reasons []string

	if !stagesEqual(oldIface.Stages, newIface.Stages) {
		reasons = append(reasons, fmt.Sprintf("stages changed: %v -> %v", oldIface.Stages, newIface.Stages))
	}
	reasons = append(reasons, diffIO("input", oldIface.Inputs, newIface.Inputs)...)
	reasons = append(reasons, diffIO("output", oldIface.Outputs, newIface.Outputs)...)
	reasons = append(reasons, diffCBuffers(oldIface.ConstantBuffer, newIface.ConstantBuffer)...)
	reasons = append(reasons, diffResources(oldIface.Resources, newIface.Resources)...)

	if len(reasons) > 0 {
		return &IncompatibleError{Reasons: reasons}
	}
	return nil
}

func stagesEqual(a, b []Stage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func diffIO(kind string, oldL, newL []IOVariable) []string {
	var reasons []string
	oldByLoc := map[int]IOVariable{}
	for _, v := range oldL {
		oldByLoc[v.Location] = v
	}
	newByLoc := map[int]IOVariable{}
	for _, v := range newL {
		newByLoc[v.Location] = v
	}
	for loc, ov := range oldByLoc {
		nv, ok := newByLoc[loc]
		if !ok {
			reasons = append(reasons, fmt.Sprintf("%s %q @%d removed", kind, ov.Name, loc))
			continue
		}
		if ov.Type != nv.Type {
			reasons = append(reasons, fmt.Sprintf("%s @%d type changed: %s -> %s", kind, loc, ov.Type, nv.Type))
		}
		if ov.Name != nv.Name {
			reasons = append(reasons, fmt.Sprintf("%s @%d renamed: %q -> %q", kind, loc, ov.Name, nv.Name))
		}
	}
	for loc, nv := range newByLoc {
		if _, ok := oldByLoc[loc]; !ok {
			reasons = append(reasons, fmt.Sprintf("%s %q @%d added", kind, nv.Name, loc))
		}
	}
	return reasons
}

func diffCBuffers(oldL, newL []ConstantBuffer) []string {
	var reasons []string
	oldByName := map[string]ConstantBuffer{}
	for _, cb := range oldL {
		oldByName[cb.Name] = cb
	}
	newByName := map[string]ConstantBuffer{}
	for _, cb := range newL {
		newByName[cb.Name] = cb
	}
	for name, ocb := range oldByName {
		ncb, ok := newByName[name]
		if !ok {
			reasons = append(reasons, fmt.Sprintf("cbuffer %q removed", name))
			continue
		}
		if ocb.Binding != ncb.Binding {
			reasons = append(reasons, fmt.Sprintf("cbuffer %q binding changed: %d -> %d", name, ocb.Binding, ncb.Binding))
		}
		if ocb.Size != ncb.Size {
			reasons = append(reasons, fmt.Sprintf("cbuffer %q size changed: %d -> %d", name, ocb.Size, ncb.Size))
		}
		reasons = append(reasons, diffFields(name, ocb.Fields, ncb.Fields)...)
	}
	for name := range newByName {
		if _, ok := oldByName[name]; !ok {
			reasons = append(reasons, fmt.Sprintf("cbuffer %q added", name))
		}
	}
	return reasons
}

func diffFields(cbName string, oldF, newF []ConstantField) []string {
	var reasons []string
	oldByName := map[string]ConstantField{}
	for _, f := range oldF {
		oldByName[f.Name] = f
	}
	newByName := map[string]ConstantField{}
	for _, f := range newF {
		newByName[f.Name] = f
	}
	for name, of := range oldByName {
		nf, ok := newByName[name]
		if !ok {
			reasons = append(reasons, fmt.Sprintf("cbuffer %q field %q removed", cbName, name))
			continue
		}
		if of.Type != nf.Type {
			reasons = append(reasons, fmt.Sprintf("cbuffer %q field %q type changed: %s -> %s", cbName, name, of.Type, nf.Type))
		}
		if of.Offset != nf.Offset {
			reasons = append(reasons, fmt.Sprintf("cbuffer %q field %q offset changed: %d -> %d", cbName, name, of.Offset, nf.Offset))
		}
	}
	for name := range newByName {
		if _, ok := oldByName[name]; !ok {
			reasons = append(reasons, fmt.Sprintf("cbuffer %q field %q added", cbName, name))
		}
	}
	return reasons
}

func diffResources(oldL, newL []ResourceBinding) []string {
	var reasons []string
	oldByName := map[string]ResourceBinding{}
	for _, r := range oldL {
		oldByName[r.Name] = r
	}
	newByName := map[string]ResourceBinding{}
	for _, r := range newL {
		newByName[r.Name] = r
	}
	for name, or := range oldByName {
		nr, ok := newByName[name]
		if !ok {
			reasons = append(reasons, fmt.Sprintf("resource %q removed", name))
			continue
		}
		if or.Binding != nr.Binding {
			reasons = append(reasons, fmt.Sprintf("resource %q binding changed: %d -> %d", name, or.Binding, nr.Binding))
		}
		if or.Type != nr.Type {
			reasons = append(reasons, fmt.Sprintf("resource %q type changed: %s -> %s", name, or.Type, nr.Type))
		}
		if !stagesEqual(or.Stages, nr.Stages) {
			reasons = append(reasons, fmt.Sprintf("resource %q stages changed: %v -> %v", name, or.Stages, nr.Stages))
		}
	}
	for name := range newByName {
		if _, ok := oldByName[name]; !ok {
			reasons = append(reasons, fmt.Sprintf("resource %q added", name))
		}
	}
	return reasons
}
