package shader

import (
	"sync"
	"sync/atomic"
)

// BindingState 保存运行期已设置的常量与资源绑定值。
// 接口不变的重载必须保留这些值。
type BindingState struct {
	constants map[string][]byte // 常量缓冲名 -> 数据
	resources map[string]uint64 // 资源绑定名 -> 句柄
}

func newBindingState() *BindingState {
	return &BindingState{
		constants: make(map[string][]byte),
		resources: make(map[string]uint64),
	}
}

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

// version 是一个不可变的管线版本：模块 + 接口 + 绑定状态。
type version struct {
	module *CompiledModule
	iface  *Interface
	state  *BindingState
}

// Pipeline 支持原子换版的热重载管线。
// 任意读取方在任意时刻只能看到某一个完整版本。
type Pipeline struct {
	mu      sync.RWMutex
	current *version
	// stale 是重载开始时的完整版本快照，仅在重载临界区内非空，
	// 供 PolicyUseOld 的读取方在重载期间使用。用 atomic 保证无锁读取安全。
	stale  atomic.Pointer[version]
	policy ReloadPolicy
}

// NewPipeline 用初始编译产物创建管线。
func NewPipeline(m *CompiledModule) (*Pipeline, error) {
	iface, err := ReflectCompiled(m)
	if err != nil {
		return nil, err
	}
	return &Pipeline{current: &version{module: m, iface: iface, state: newBindingState()}}, nil
}

// Frame 是一次渲染提交看到的一致性快照。
type Frame struct {
	iface *Interface
	state *BindingState
}

// BeginFrame 获取当前版本的一致性快照；快照在帧生命周期内不受重载影响。
//
// 重载临界区内的行为由 ReloadPolicy 决定：
//   - PolicyWait：阻塞至重载完成，返回重载后的完整版本；
//   - PolicyUseOld：立即返回重载开始前的旧版本快照，绝不阻塞。
func (p *Pipeline) BeginFrame() *Frame {
	if p.policy == PolicyUseOld {
		if p.mu.TryRLock() {
			defer p.mu.RUnlock()
			return &Frame{iface: p.current.iface, state: p.current.state}
		}
		// 重载临界区内：读旧版本快照（完整版本，不会新旧混合）。
		if v := p.stale.Load(); v != nil {
			return &Frame{iface: v.iface, state: v.state}
		}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return &Frame{iface: p.current.iface, state: p.current.state}
}

// Interface 返回帧快照对应的接口。
func (f *Frame) Interface() *Interface { return f.iface }

// SetConstant 设置常量缓冲数据（按名称）。
func (p *Pipeline) SetConstant(name string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, cb := range p.current.iface.ConstantBuffers {
		if cb.Name == name {
			buf := make([]byte, len(data))
			copy(buf, data)
			p.current.state.constants[name] = buf
			return nil
		}
	}
	return &IncompatibleError{Module: p.current.module.name, Detail: "unknown constant buffer " + name}
}

// Constant 读取已设置的常量数据。
func (p *Pipeline) Constant(name string) ([]byte, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	v, ok := p.current.state.constants[name]
	return v, ok
}

// BindResource 绑定资源句柄（按名称）。
func (p *Pipeline) BindResource(name string, handle uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.current.iface.Resources {
		if r.Name == name {
			p.current.state.resources[name] = handle
			return nil
		}
	}
	return &IncompatibleError{Module: p.current.module.name, Detail: "unknown resource " + name}
}

// Resource 读取已绑定的资源句柄。
func (p *Pipeline) Resource(name string) (uint64, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	v, ok := p.current.state.resources[name]
	return v, ok
}
