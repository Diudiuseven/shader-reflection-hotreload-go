package shader

import (
	"errors"
	"fmt"
)

// CompileError 表示着色器编译失败（如源被截断、语法错误）。
// 热重载遇到该错误时旧版本继续可用。
type CompileError struct {
	Source  string // 来源标识（文件路径或模块名）
	Line    int
	Message string
}

func (e *CompileError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("shader compile error in %s:%d: %s", e.Source, e.Line, e.Message)
	}
	return fmt.Sprintf("shader compile error in %s: %s", e.Source, e.Message)
}

// IncompatibleReason 枚举接口不兼容的具体原因。
type IncompatibleReason int

const (
	ReasonBindingChanged IncompatibleReason = iota // 绑定位置变化
	ReasonTypeChanged                              // 类型变化
	ReasonStageChanged                             // 所需阶段变化
	ReasonMemberRemoved                            // 既有接口成员被移除
	ReasonLayoutChanged                            // 布局（如常量缓冲大小）变化
)

func (r IncompatibleReason) String() string {
	switch r {
	case ReasonBindingChanged:
		return "binding location changed"
	case ReasonTypeChanged:
		return "type changed"
	case ReasonStageChanged:
		return "required stages changed"
	case ReasonMemberRemoved:
		return "interface member removed"
	case ReasonLayoutChanged:
		return "layout changed"
	}
	return "unknown"
}

// IncompatibleError 表示新反射接口与既有绑定状态不兼容。
// 不得静默替换；调用方可用 errors.As 区分该错误并读取具体原因列表。
type IncompatibleError struct {
	Module  string
	Reasons []IncompatibleReason
	Detail  string
}

func (e *IncompatibleError) Error() string {
	return fmt.Sprintf("shader module %q interface incompatible: %s (%s)",
		e.Module, e.Detail, reasonsString(e.Reasons))
}

func reasonsString(rs []IncompatibleReason) string {
	s := ""
	for i, r := range rs {
		if i > 0 {
			s += ", "
		}
		s += r.String()
	}
	return s
}

// IsCompileError 报告 err 是否为编译失败。
func IsCompileError(err error) bool {
	var ce *CompileError
	return errors.As(err, &ce)
}

// IsIncompatible 报告 err 是否为接口不兼容。
func IsIncompatible(err error) bool {
	var ie *IncompatibleError
	return errors.As(err, &ie)
}
