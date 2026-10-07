package shader

import "fmt"

// BindingState 保存运行期已设置的常量与资源绑定值。
// 重载且接口兼容时，该状态被整体保留，不因重载丢失。
type BindingState struct {
	constants map[string][]byte // 常量缓冲名 -> 数据（长度固定为缓冲大小）
	resources map[string]int    // 资源名 -> 绑定句柄
}

func newBindingState() *BindingState {
	return &BindingState{
		constants: map[string][]byte{},
		resources: map[string]int{},
	}
}

// clone 深拷贝绑定状态，用于重载时迁移到新版本。
func (s *BindingState) clone() *BindingState {
	c := newBindingState()
	for k, v := range s.constants {
		buf := make([]byte, len(v))
		copy(buf, v)
		c.constants[k] = buf
	}
	for k, v := range s.resources {
		c.resources[k] = v
	}
	return c
}

// ShaderModule 是一个已加载的着色器模块：编译产物 + 接口 + 绑定状态。
// 模块一旦发布即不可变地对外提供接口；绑定状态通过内部锁保护。
type ShaderModule struct {
	module *Module
	iface  *ShaderInterface
	state  *BindingState
}

// NewShaderModule 编译源码并创建着色器模块（既有加载通路）。
func NewShaderModule(src []byte) (*ShaderModule, error) {
	mod, err := Compile(src)
	if err != nil {
		return nil, err
	}
	return &ShaderModule{module: mod, iface: mod.iface, state: newBindingState()}, nil
}

// Interface 返回模块当前生效的接口。
func (m *ShaderModule) Interface() *ShaderInterface {
	cp := *m.iface
	return &cp
}

// SourceSum 返回当前版本的源码摘要。
func (m *ShaderModule) SourceSum() [16]byte { return m.module.sourceSum }

// SetConstant 设置常量缓冲数据。缓冲必须存在，数据长度不得超过缓冲大小。
func (m *ShaderModule) SetConstant(bufferName string, data []byte) error {
	var cb *ConstantBuffer
	for i := range m.iface.ConstantBuffer {
		if m.iface.ConstantBuffer[i].Name == bufferName {
			cb = &m.iface.ConstantBuffer[i]
			break
		}
	}
	if cb == nil {
		return fmt.Errorf("set constant: no cbuffer %q in interface", bufferName)
	}
	if len(data) > cb.Size {
		return fmt.Errorf("set constant: data %d bytes exceeds cbuffer %q size %d", len(data), bufferName, cb.Size)
	}
	buf := make([]byte, cb.Size)
	copy(buf, data)
	m.state.constants[bufferName] = buf
	return nil
}

// SetResource 绑定资源。资源名必须存在于接口中。
func (m *ShaderModule) SetResource(name string, handle int) error {
	for _, r := range m.iface.Resources {
		if r.Name == name {
			m.state.resources[name] = handle
			return nil
		}
	}
	return fmt.Errorf("set resource: no resource %q in interface", name)
}

// Constant 读取已设置的常量数据（用于测试与校验）。
func (m *ShaderModule) Constant(bufferName string) ([]byte, bool) {
	v, ok := m.state.constants[bufferName]
	if !ok {
		return nil, false
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, true
}

// Resource 读取已绑定的资源句柄。
func (m *ShaderModule) Resource(name string) (int, bool) {
	v, ok := m.state.resources[name]
	return v, ok
}
