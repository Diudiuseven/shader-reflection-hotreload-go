package shader

import (
	"strconv"
	"strings"
)

// ReflectSource 从着色器源文本反射出对外接口。
// 反射是确定性的：同一输入总是产生相同顺序、相同内容的 Interface。
func ReflectSource(source string) (*Interface, error) {
	iface, err := parseSource(source)
	if err != nil {
		return nil, err
	}
	return iface, nil
}

// ReflectCompiled 从编译产物反射接口。
// 编译产物内嵌了反射信息，因此结果与实际生效的绑定一致。
func ReflectCompiled(c *CompiledModule) (*Interface, error) {
	if c == nil || c.iface == nil {
		return nil, &CompileError{Source: "<compiled>", Message: "empty compiled module"}
	}
	cp := *c.iface
	return &cp, nil
}

// 着色器源文本格式（行式，确定性解析）：
//
//	#stage vertex|fragment|compute          声明模块包含的阶段（至少一条）
//	in <type> <name> @location(<n>)         阶段输入
//	out <type> <name> @location(<n>)        阶段输出
//	const <name> @binding(<n>) size(<bytes>) stages(a,b)
//	texture2d|sampler|storage_buffer <name> @binding(<n>) stages(a,b)
//
// 空行与 // 注释被忽略。任何行不完整（如源被截断）都会报带行号的编译错误。
func parseSource(source string) (*Interface, error) {
	iface := &Interface{}
	seenStage := map[Stage]bool{}
	lines := strings.Split(source, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		lineNo := i + 1
		fail := func(msg string) (*Interface, error) {
			return nil, &CompileError{Source: "<source>", Line: lineNo, Message: msg}
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "#stage":
			if len(fields) != 2 {
				return fail("malformed #stage directive")
			}
			st, ok := parseStage(fields[1])
			if !ok {
				return fail("unknown stage " + fields[1])
			}
			if !seenStage[st] {
				seenStage[st] = true
				iface.Stages = append(iface.Stages, st)
			}
		case "in", "out":
			if len(fields) != 4 {
				return fail("malformed " + fields[0] + " declaration (truncated?)")
			}
			loc, ok := parseAttrInt(fields[3], "@location(")
			if !ok {
				return fail("missing or invalid @location for " + fields[2])
			}
			v := IOVariable{Name: fields[2], Type: DataType(fields[1]), Location: loc}
			if fields[0] == "in" {
				iface.Inputs = append(iface.Inputs, v)
			} else {
				iface.Outputs = append(iface.Outputs, v)
			}
		case "const":
			if len(fields) != 5 {
				return fail("malformed const declaration (truncated?)")
			}
			binding, ok := parseAttrInt(fields[2], "@binding(")
			if !ok {
				return fail("missing or invalid @binding for " + fields[1])
			}
			size, ok := parseAttrInt(fields[3], "size(")
			if !ok {
				return fail("missing or invalid size for " + fields[1])
			}
			st, ok := parseStagesAttr(fields[4])
			if !ok {
				return fail("missing or invalid stages for " + fields[1])
			}
			iface.ConstantBuffers = append(iface.ConstantBuffers,
				ConstantBuffer{Name: fields[1], Binding: binding, Size: size, Stages: st})
		case "texture2d", "sampler", "storage_buffer":
			if len(fields) != 4 {
				return fail("malformed resource declaration (truncated?)")
			}
			binding, ok := parseAttrInt(fields[2], "@binding(")
			if !ok {
				return fail("missing or invalid @binding for " + fields[1])
			}
			st, ok := parseStagesAttr(fields[3])
			if !ok {
				return fail("missing or invalid stages for " + fields[1])
			}
			iface.Resources = append(iface.Resources, ResourceBinding{
				Name: fields[1], Kind: ResourceKind(fields[0]), Binding: binding, Stages: st})
		default:
			return fail("unrecognized directive " + fields[0])
		}
	}
	if len(iface.Stages) == 0 {
		return nil, &CompileError{Source: "<source>", Message: "no #stage directive (empty or truncated source?)"}
	}
	return iface, nil
}

func parseStage(s string) (Stage, bool) {
	switch s {
	case "vertex":
		return StageVertex, true
	case "fragment":
		return StageFragment, true
	case "compute":
		return StageCompute, true
	}
	return 0, false
}

// parseAttrInt 解析形如 prefix + "<digits>)" 的属性。
func parseAttrInt(s, prefix string) (int, bool) {
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, ")") {
		return 0, false
	}
	n, err := strconv.Atoi(s[len(prefix) : len(s)-1])
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// parseStagesAttr 解析 stages(vertex,fragment) 形式的属性。
func parseStagesAttr(s string) (Stages, bool) {
	const prefix = "stages("
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, ")") {
		return 0, false
	}
	body := s[len(prefix) : len(s)-1]
	if body == "" {
		return 0, false
	}
	var m Stages
	for _, name := range strings.Split(body, ",") {
		st, ok := parseStage(name)
		if !ok {
			return 0, false
		}
		m |= 1 << st
	}
	return m, true
}
