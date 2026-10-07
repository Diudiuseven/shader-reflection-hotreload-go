package shader

import (
	"crypto/md5"
	"fmt"
)

// Module 是着色器的编译产物，内嵌反射信息。
type Module struct {
	Bytecode  []byte
	iface     *ShaderInterface
	sourceSum [16]byte
}

// Compile 将着色器源码编译为 Module。
// 源码非法（如被截断、语法错误）时返回错误，不产生任何半成品。
func Compile(src []byte) (*Module, error) {
	if len(src) == 0 {
		return nil, fmt.Errorf("compile: empty source")
	}
	iface, err := ReflectFromSource(src)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	bc := make([]byte, len(src))
	copy(bc, src)
	return &Module{
		Bytecode:  bc,
		iface:     iface,
		sourceSum: md5.Sum(src),
	}, nil
}

// SourceSum 返回源码摘要，用于识别重复改动（去抖）。
func (m *Module) SourceSum() [16]byte { return m.sourceSum }
