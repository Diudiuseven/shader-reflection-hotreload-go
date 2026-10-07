package shader

// ReloadPolicy 决定重载期间新到帧的行为。
type ReloadPolicy int

const (
	// PolicyUseOld 重载期间新帧继续走旧版本（默认，确定且不阻塞）。
	PolicyUseOld ReloadPolicy = iota
	// PolicyWait 新帧等待重载完成后走新版本。
	PolicyWait
)

// ReloadResult 描述一次重载尝试的结果。
type ReloadResult int

const (
	ReloadSwapped       ReloadResult = iota // 接口兼容，已原子换版
	ReloadIncompatible                      // 接口不兼容，旧版本保留
	ReloadCompileFailed                     // 编译失败，旧版本保留
)

// Reloader 对单个 Pipeline 执行原子热重载。
type Reloader struct {
	pipeline *Pipeline
	compiler Compiler
	policy   ReloadPolicy
}

// NewReloader 构造重载器，并把重载策略应用到管线。
func NewReloader(p *Pipeline, c Compiler, policy ReloadPolicy) *Reloader {
	p.mu.Lock()
	p.policy = policy
	p.mu.Unlock()
	return &Reloader{pipeline: p, compiler: c, policy: policy}
}

// Reload 用新源文本执行一次重载。
// 编译失败或接口不兼容时旧版本继续可用；兼容时原子换版并保留绑定状态。
// 整个重载（编译+校验+换版）在写锁临界区内完成，读取方按 ReloadPolicy
// 要么等待、要么走重载前的旧版本快照，行为确定。
func (r *Reloader) Reload(source string) (ReloadResult, error) {
	r.pipeline.mu.Lock()
	defer r.pipeline.mu.Unlock()
	r.pipeline.stale.Store(r.pipeline.current)
	defer r.pipeline.stale.Store(nil)
	m, err := r.compiler.Compile(r.pipeline.current.module.name, source)
	if err != nil {
		return ReloadCompileFailed, err
	}
	return r.swapLocked(m)
}

// ReloadCompiled 用已编译产物执行一次重载。
func (r *Reloader) ReloadCompiled(m *CompiledModule) (ReloadResult, error) {
	r.pipeline.mu.Lock()
	defer r.pipeline.mu.Unlock()
	r.pipeline.stale.Store(r.pipeline.current)
	defer r.pipeline.stale.Store(nil)
	return r.swapLocked(m)
}

// swapLocked 校验兼容性并在兼容时原子换版。调用方必须持有写锁。
func (r *Reloader) swapLocked(m *CompiledModule) (ReloadResult, error) {
	iface, err := ReflectCompiled(m)
	if err != nil {
		return ReloadCompileFailed, err
	}
	cur := r.pipeline.current
	if cerr := CheckCompatible(cur.iface, iface); cerr != nil {
		return ReloadIncompatible, cerr
	}
	// 接口兼容：保留既有绑定状态，原子换版。
	r.pipeline.current = &version{module: m, iface: iface, state: cur.state}
	return ReloadSwapped, nil
}

// CheckCompatible 判定新接口与既有接口是否兼容。
// 绑定位置、类型、所需阶段或布局变化均判定为不兼容；允许新增成员。
func CheckCompatible(old, new *Interface) error {
	var reasons []IncompatibleReason
	detail := ""
	add := func(r IncompatibleReason, d string) {
		reasons = append(reasons, r)
		if detail != "" {
			detail += "; "
		}
		detail += d
	}
	checkIO("input", old.Inputs, new.Inputs, add)
	checkIO("output", old.Outputs, new.Outputs, add)
	checkConstants(old.ConstantBuffers, new.ConstantBuffers, add)
	checkResources(old.Resources, new.Resources, add)
	if len(reasons) > 0 {
		return &IncompatibleError{Reasons: reasons, Detail: detail}
	}
	return nil
}

func checkIO(kind string, old, new []IOVariable, add func(IncompatibleReason, string)) {
	byName := make(map[string]IOVariable, len(new))
	for _, v := range new {
		byName[v.Name] = v
	}
	for _, o := range old {
		n, ok := byName[o.Name]
		if !ok {
			add(ReasonMemberRemoved, kind+" "+o.Name+" removed")
			continue
		}
		if n.Location != o.Location {
			add(ReasonBindingChanged, kind+" "+o.Name+" location changed")
		}
		if n.Type != o.Type {
			add(ReasonTypeChanged, kind+" "+o.Name+" type changed")
		}
	}
}

func checkConstants(old, new []ConstantBuffer, add func(IncompatibleReason, string)) {
	byName := make(map[string]ConstantBuffer, len(new))
	for _, v := range new {
		byName[v.Name] = v
	}
	for _, o := range old {
		n, ok := byName[o.Name]
		if !ok {
			add(ReasonMemberRemoved, "constant buffer "+o.Name+" removed")
			continue
		}
		if n.Binding != o.Binding {
			add(ReasonBindingChanged, "constant buffer "+o.Name+" binding changed")
		}
		if n.Size != o.Size {
			add(ReasonLayoutChanged, "constant buffer "+o.Name+" size changed")
		}
		if n.Stages != o.Stages {
			add(ReasonStageChanged, "constant buffer "+o.Name+" stages changed")
		}
	}
}

func checkResources(old, new []ResourceBinding, add func(IncompatibleReason, string)) {
	byName := make(map[string]ResourceBinding, len(new))
	for _, v := range new {
		byName[v.Name] = v
	}
	for _, o := range old {
		n, ok := byName[o.Name]
		if !ok {
			add(ReasonMemberRemoved, "resource "+o.Name+" removed")
			continue
		}
		if n.Binding != o.Binding {
			add(ReasonBindingChanged, "resource "+o.Name+" binding changed")
		}
		if n.Kind != o.Kind {
			add(ReasonTypeChanged, "resource "+o.Name+" kind changed")
		}
		if n.Stages != o.Stages {
			add(ReasonStageChanged, "resource "+o.Name+" stages changed")
		}
	}
}
