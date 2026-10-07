package shader

import (
	"sync"
	"sync/atomic"
)

// ReloadPolicy 决定重载期间新到帧的行为。
type ReloadPolicy int

const (
	// PolicyUseOld 重载期间新帧继续使用旧版本（默认，新帧从不阻塞）。
	PolicyUseOld ReloadPolicy = iota
	// PolicyWait 重载期间新帧等待重载完成，随后使用新版本。
	PolicyWait
)

// Reloader 管理着色器模块的热重载。
// 原子性：当前版本指针原子替换，任何读取方在任意时刻只能看到某一个完整版本。
// 并发性：重载由 reloadMu 串行化；帧提交按策略选择快照或等待。
type Reloader struct {
	policy ReloadPolicy

	cur      atomic.Pointer[ShaderModule]
	reloadMu sync.Mutex   // 串行化重载
	frameMu  sync.RWMutex // 仅 PolicyWait 使用：重载持写锁，帧持读锁
}

// NewReloader 创建重载器，初始版本为 init。
func NewReloader(init *ShaderModule, policy ReloadPolicy) *Reloader {
	r := &Reloader{policy: policy}
	r.cur.Store(init)
	return r
}

// Current 返回当前生效版本，任意时刻只返回某一个完整版本。
func (r *Reloader) Current() *ShaderModule { return r.cur.Load() }

// Reload 用新源码重载：
//  1. 编译失败：返回编译错误，旧版本继续可用；
//  2. 接口不兼容：返回 *IncompatibleError，旧版本继续可用；
//  3. 兼容：绑定状态整体迁移，原子替换为新版本。
//
// 源码与当前版本相同（重复改动）时为空操作，返回 nil。
func (r *Reloader) Reload(src []byte) error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	old := r.cur.Load()
	mod, err := Compile(src)
	if err != nil {
		return err // 编译失败：旧版本不动
	}
	if mod.sourceSum == old.module.sourceSum {
		return nil // 内容未变：去抖，空操作
	}
	if err := CheckCompatible(old.iface, mod.iface); err != nil {
		return err // 不兼容：旧版本不动
	}
	next := &ShaderModule{module: mod, iface: mod.iface, state: old.state.clone()}

	if r.policy == PolicyWait {
		r.frameMu.Lock()
		r.cur.Store(next)
		r.frameMu.Unlock()
	} else {
		r.cur.Store(next)
	}
	return nil
}

// BeginFrame 开始一次帧提交，返回该帧应使用的版本快照。
// 快照在帧存续期间保证完整一致；PolicyWait 下若正在重载则阻塞至重载完成。
func (r *Reloader) BeginFrame() *Frame {
	f := &Frame{r: r}
	if r.policy == PolicyWait {
		r.frameMu.RLock()
		f.holdsLock = true
	}
	f.module = r.cur.Load()
	return f
}

// Frame 是一次帧提交的版本快照。
type Frame struct {
	r         *Reloader
	module    *ShaderModule
	holdsLock bool
}

// Module 返回该帧使用的模块版本。
func (f *Frame) Module() *ShaderModule { return f.module }

// End 结束帧提交，释放帧持有的资源。
func (f *Frame) End() {
	if f.holdsLock {
		f.r.frameMu.RUnlock()
		f.holdsLock = false
	}
}
