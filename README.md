# shader-reflection-hotreload-go

着色器模块的接口反射与运行期热重载库。在保留既有加载通路（`Compiler` → `CompiledModule` → `Pipeline`）的基础上，补充了接口反射、兼容性判定与原子热重载能力。

## 着色器源格式

本库使用一种行式的确定性源格式（`FakeCompiler` 直接解析它；接入真实编译器时替换 `Compiler` 实现即可）：

```
#stage vertex
#stage fragment
in float3 pos @location(0)
in float2 uv @location(1)
out float4 color @location(0)
const MVP @binding(0) size(64) stages(vertex)
texture2d albedo @binding(1) stages(fragment)
sampler samp @binding(2) stages(fragment)
```

## 反射语义

- `ReflectSource(source)` 从源文本反射；`ReflectCompiled(module)` 从编译产物反射。编译产物内嵌反射信息，因此**反射结果与实际生效的绑定一致**。
- 反射是**确定且可重复**的：同一输入总是产生相同顺序、相同内容的 `Interface`（声明顺序即输出顺序）。
- `Interface` 包含：阶段列表 `Stages`、输入 `Inputs`、输出 `Outputs`（名称/类型/location）、常量缓冲 `ConstantBuffers`（名称/binding/字节大小/所需阶段）、资源绑定 `Resources`（名称/类别/binding/所需阶段）。
- 源不合法（包括被截断、缺少 `#stage`）时返回 `*CompileError`，携带行号，可用 `IsCompileError` 判定。

## 兼容性判定

`CheckCompatible(old, new)` 判定新接口能否替换既有绑定状态。以下变化判定为**不兼容**，返回 `*IncompatibleError`（可用 `IsIncompatible` / `errors.As` 区分，并携带 `Reasons` 列表）：

| 变化 | 原因枚举 |
|---|---|
| 绑定位置 / location 变化 | `ReasonBindingChanged` |
| 成员类型或资源类别变化 | `ReasonTypeChanged` |
| 所需阶段变化 | `ReasonStageChanged` |
| 既有成员被移除 | `ReasonMemberRemoved` |
| 常量缓冲大小（布局）变化 | `ReasonLayoutChanged` |

允许的变化：仅新增成员、仅修改实现（注释/函数体等不影响接口的内容）。

不兼容时**绝不静默替换**：`Reloader` 返回 `ReloadIncompatible` 与 `*IncompatibleError`，旧版本继续服务。

## 热重载语义

- **原子性**：编译失败（`ReloadCompileFailed`）或校验失败（`ReloadIncompatible`）时旧版本继续可用；任何读取方在任意时刻只能看到某一个完整版本，不会新旧混合。
- **状态保留**：接口兼容的重载只替换实现，运行期已设置的常量（`SetConstant`）与资源绑定（`BindResource`）全部保留。
- **并发策略**（`NewReloader(pipeline, compiler, policy)`）：
  - `PolicyUseOld`（默认）：重载临界区内新到的帧立即拿到重载前的旧版本快照，不阻塞；
  - `PolicyWait`：新到的帧阻塞至重载完成，拿到新版本。
  - 正在提交的帧已持有快照，不受重载影响。
- **异常链路**：着色器被截断 → `CompileError`；连续多次改动 → 每次独立判定；改动后回退 → 与当前生效版本重新比对，可正常换回。重载不在管线中残留任何临时状态。

## 变更监听与去抖

`NewWatcher(reloader, debounce, read)` 监听来源变更：

- 去抖窗口内的重复 `Notify` 只触发一次重载；
- 每次触发读取最新源文本，事件按触发顺序记录在 `Events()` 中，结果可复现；
- `Close()` 清理所有挂起的定时器，不残留临时状态；Close 后的 `Notify` 被忽略。

## 测试

```
go test -race ./...
```

覆盖：接口不变的重载（状态保留）、接口不兼容（绑定/类型/阶段变化）、编译失败（截断）、连续改动与回退、重载与渲染提交并发、重载策略、监听器去抖与关闭。
