package shader

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// 着色器源格式（行导向 DSL，用于演示反射语义）：
//   #stage vertex|fragment|compute          声明阶段（至少一个）
//   in  <type> <name> @<location>           阶段输入
//   out <type> <name> @<location>           阶段输出
//   cbuffer <name> @<binding> {             常量缓冲，字段逐行声明，以 } 结束
//     <type> <field>;
//   }
//   uniform <type> <name> @<binding> [stage,stage]   资源绑定，阶段可省略（默认全部阶段）

// dataTypeInfo 返回类型的字节大小与对齐。
func dataTypeInfo(t DataType) (size, align int, err error) {
	switch t {
	case TypeFloat, TypeInt:
		return 4, 4, nil
	case TypeVec2:
		return 8, 8, nil
	case TypeVec3:
		return 16, 16, nil // std140 风格：vec3 按 vec4 对齐
	case TypeVec4:
		return 16, 16, nil
	case TypeMat4:
		return 64, 16, nil
	}
	return 0, 0, fmt.Errorf("unknown data type %q", t)
}

func alignUp(v, a int) int { return (v + a - 1) / a * a }

var stageOrder = map[Stage]int{StageVertex: 0, StageFragment: 1, StageCompute: 2}

// ReflectFromSource 从着色器源码反射出对外接口。
// 结果按绑定位置与名称排序，保证同一输入总是得到同一结果。
func ReflectFromSource(src []byte) (*ShaderInterface, error) {
	iface := &ShaderInterface{}
	seenStage := map[Stage]bool{}
	lines := strings.Split(string(src), "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		i++
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "#stage":
			if len(fields) != 2 {
				return nil, fmt.Errorf("line %d: malformed #stage directive", i)
			}
			st := Stage(fields[1])
			if _, ok := stageOrder[st]; !ok {
				return nil, fmt.Errorf("line %d: unknown stage %q", i, fields[1])
			}
			if !seenStage[st] {
				seenStage[st] = true
				iface.Stages = append(iface.Stages, st)
			}
		case "in", "out":
			v, err := parseIO(fields)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i, err)
			}
			if fields[0] == "in" {
				iface.Inputs = append(iface.Inputs, v)
			} else {
				iface.Outputs = append(iface.Outputs, v)
			}
		case "cbuffer":
			cb, consumed, err := parseCBuffer(fields, lines, i)
			if err != nil {
				return nil, err
			}
			i += consumed
			iface.ConstantBuffer = append(iface.ConstantBuffer, cb)
		case "uniform":
			r, err := parseResource(fields, iface.Stages)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i, err)
			}
			iface.Resources = append(iface.Resources, r)
		default:
			return nil, fmt.Errorf("line %d: unrecognized statement %q", i, fields[0])
		}
	}
	if len(iface.Stages) == 0 {
		return nil, fmt.Errorf("no #stage directive found")
	}
	sortInterface(iface)
	if err := validateInterface(iface); err != nil {
		return nil, err
	}
	return iface, nil
}

func parseIO(fields []string) (IOVariable, error) {
	var v IOVariable
	if len(fields) != 4 || !strings.HasPrefix(fields[3], "@") {
		return v, fmt.Errorf("malformed io declaration, want: in|out <type> <name> @<location>")
	}
	loc, err := strconv.Atoi(fields[3][1:])
	if err != nil || loc < 0 {
		return v, fmt.Errorf("invalid location %q", fields[3])
	}
	t := DataType(fields[1])
	if _, _, err := dataTypeInfo(t); err != nil {
		return v, err
	}
	return IOVariable{Name: fields[2], Type: t, Location: loc}, nil
}

// parseCBuffer 解析 cbuffer 声明。lines 为全部行，start 为 cbuffer 行的下一行下标。
// 返回解析结果与消耗的行数（不含 cbuffer 行本身）。
func parseCBuffer(header []string, lines []string, start int) (ConstantBuffer, int, error) {
	var cb ConstantBuffer
	// 支持 "cbuffer Name @0 {" 或 "cbuffer Name @0" 后换行 "{"
	if len(header) < 3 || !strings.HasPrefix(header[2], "@") {
		return cb, 0, fmt.Errorf("line %d: malformed cbuffer header", start)
	}
	binding, err := strconv.Atoi(header[2][1:])
	if err != nil || binding < 0 {
		return cb, 0, fmt.Errorf("line %d: invalid cbuffer binding %q", start, header[2])
	}
	cb.Name, cb.Binding = header[1], binding
	open := len(header) == 4 && header[3] == "{"
	offset := 0
	consumed := 0
	for {
		if open {
			break
		}
		if start+consumed >= len(lines) {
			return cb, 0, fmt.Errorf("line %d: cbuffer %q truncated: missing '{'", start, cb.Name)
		}
		if strings.TrimSpace(lines[start+consumed]) == "{" {
			consumed++
			break
		}
		return cb, 0, fmt.Errorf("line %d: expected '{' after cbuffer header", start+consumed+1)
	}
	for {
		if start+consumed >= len(lines) {
			return cb, 0, fmt.Errorf("cbuffer %q truncated: missing '}'", cb.Name)
		}
		line := strings.TrimSpace(lines[start+consumed])
		consumed++
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if line == "}" {
			break
		}
		f := strings.Fields(strings.TrimSuffix(line, ";"))
		if len(f) != 2 {
			return cb, 0, fmt.Errorf("line %d: malformed cbuffer field %q", start+consumed, line)
		}
		size, align, err := dataTypeInfo(DataType(f[0]))
		if err != nil {
			return cb, 0, fmt.Errorf("line %d: %w", start+consumed, err)
		}
		offset = alignUp(offset, align)
		cb.Fields = append(cb.Fields, ConstantField{Name: f[1], Type: DataType(f[0]), Offset: offset, Size: size})
		offset += size
	}
	cb.Size = alignUp(offset, 16)
	return cb, consumed, nil
}

func parseResource(fields []string, declared []Stage) (ResourceBinding, error) {
	var r ResourceBinding
	if len(fields) < 4 || !strings.HasPrefix(fields[3], "@") {
		return r, fmt.Errorf("malformed uniform, want: uniform <type> <name> @<binding> [stages]")
	}
	binding, err := strconv.Atoi(fields[3][1:])
	if err != nil || binding < 0 {
		return r, fmt.Errorf("invalid binding %q", fields[3])
	}
	t := DataType(fields[1])
	switch t {
	case TypeSampler2D, TypeTexture2D:
	default:
		return r, fmt.Errorf("invalid resource type %q", t)
	}
	r = ResourceBinding{Name: fields[2], Type: t, Binding: binding}
	if len(fields) == 5 {
		for _, s := range strings.Split(strings.Trim(fields[4], "[]"), ",") {
			st := Stage(strings.TrimSpace(s))
			if _, ok := stageOrder[st]; !ok {
				return r, fmt.Errorf("unknown stage %q in resource stage list", s)
			}
			r.Stages = append(r.Stages, st)
		}
	} else {
		r.Stages = append(r.Stages, declared...)
	}
	sort.Slice(r.Stages, func(a, b int) bool { return stageOrder[r.Stages[a]] < stageOrder[r.Stages[b]] })
	return r, nil
}

// sortInterface 排序，保证反射结果可重复。
func sortInterface(iface *ShaderInterface) {
	sort.Slice(iface.Stages, func(a, b int) bool { return stageOrder[iface.Stages[a]] < stageOrder[iface.Stages[b]] })
	sort.Slice(iface.Inputs, func(a, b int) bool { return iface.Inputs[a].Location < iface.Inputs[b].Location })
	sort.Slice(iface.Outputs, func(a, b int) bool { return iface.Outputs[a].Location < iface.Outputs[b].Location })
	sort.Slice(iface.ConstantBuffer, func(a, b int) bool { return iface.ConstantBuffer[a].Binding < iface.ConstantBuffer[b].Binding })
	sort.Slice(iface.Resources, func(a, b int) bool {
		if iface.Resources[a].Binding != iface.Resources[b].Binding {
			return iface.Resources[a].Binding < iface.Resources[b].Binding
		}
		return iface.Resources[a].Name < iface.Resources[b].Name
	})
}

// validateInterface 校验反射结果内部一致性（绑定冲突等）。
func validateInterface(iface *ShaderInterface) error {
	used := map[int]string{}
	for _, cb := range iface.ConstantBuffer {
		if prev, ok := used[cb.Binding]; ok {
			return fmt.Errorf("binding %d used by both %q and cbuffer %q", cb.Binding, prev, cb.Name)
		}
		used[cb.Binding] = "cbuffer " + cb.Name
	}
	for _, r := range iface.Resources {
		if prev, ok := used[r.Binding]; ok {
			return fmt.Errorf("binding %d used by both %q and resource %q", r.Binding, prev, r.Name)
		}
		used[r.Binding] = "resource " + r.Name
	}
	return nil
}

// ReflectFromModule 从编译产物反射出对外接口。
// 编译产物内嵌反射信息，结果与 ReflectFromSource 一致。
func ReflectFromModule(m *Module) (*ShaderInterface, error) {
	if m == nil || m.iface == nil {
		return nil, fmt.Errorf("module has no embedded reflection info")
	}
	cp := *m.iface
	return &cp, nil
}
