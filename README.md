# shader-reflection-hotreload-go

着色器模块的接口反射与运行期热重载库。在保留既有加载通路（`NewShaderModule`）的基础上，
补充接口反射、兼容性判定与原子热重载能力。

## 着色器源格式

行导向 DSL，用于演示反射语义：

```
#stage vertex
#stage fragment
in  vec3 position @0            // 阶段输入：类型 名称 @location
out vec4 color @0               // 阶段输出
cbuffer Camera @0 {             // 常量缓冲：名称 @binding
    mat4 viewProj;
    vec3 eye;
}
uniform texture2D albedo @1 [fragment]   // 资源：类型 名称 @binding [阶段列表]
uniform sampler2D samp @2                // 阶段列表可省略，默认覆盖全部已声明阶段
```

## 反射语义（reflect.go）

- `ReflectFromSource(src)` 从源码反射；`ReflectFromModule(mod)` 从编译产物反射。
  编译产物内嵌反射信息，两条通路的结果**完全一致**。
- 反射结果描述：阶段列表、输入/输出（名称、类型、location）、常量缓冲
  （名称、binding、字段名/类型/偏移/大小、总大小）、资源绑定（名称、类型、binding、所需阶段）。
- 字段布局按 std140 风格计算：`vec3` 按 16 字节对齐，缓冲总大小对齐到 16。
- 结果按 location/binding/名称排序，**同一输入必然得到同一结果**（可重复）。
- 反射即校验：绑定号冲突、缺失 `#stage`、被截断的 `cbuffer` 等都会在反射阶段报错，
  因此反射结果必然与实际生效的绑定一致。

## 兼容性判定（compat.go）

`CheckCompatible(old, new)` 判定新旧接口是否兼容。以下变化均判定为**不兼容**：

- 阶段集合变化；
- 输入/输出的 location、类型、名称变化，或增删；
- 常量缓冲的 binding、大小、字段（名称/类型/偏移）变化，或增删；
- 资源绑定的 binding、类型、所需阶段变化，或增删。

不兼容时返回 `*IncompatibleError`（可用 `errors.As` 与编译错误区分），
其中 `Reasons` 列出全部不兼容点。不兼容时**绝不静默替换**。

## 热重载（reload.go）

```go
r := shader.NewReloader(initModule, shader.PolicyUseOld)
err := r.Reload(newSource)   // 编译 -> 兼容判定 -> 原子替换
cur := r.Current()           // 任意时刻只返回某一个完整版本
```

- **原子性**：编译失败或接口不兼容时旧版本继续可用；替换通过原子指针完成，
  读取方不会看到新旧混合状态。
- **状态保留**：接口兼容的重载只更新实现，运行期已设置的常量与资源绑定
  （`SetConstant`/`SetResource`）整体迁移到新版本，不丢失。
- **去重**：源码摘要与当前版本相同则为空操作，重复改动不触发多余替换。
- **串行化**：多次重载内部串行执行，连续改动、改动后回退等场景结果确定。

### 重载与帧提交并发

```go
f := r.BeginFrame()   // 获取帧快照
m := f.Module()       // 帧存续期间快照不变
f.End()
```

配置项 `ReloadPolicy`：

| 策略 | 行为 |
|---|---|
| `PolicyUseOld`（默认） | 重载期间新到的帧直接使用当前快照（可能是旧版本），从不阻塞 |
| `PolicyWait` | 重载替换期间新到的帧阻塞至替换完成，随后使用新版本；进行中的帧不受影响 |

两种策略下行为都是确定的：任何一帧只能看到某一个完整版本。

## 变更监听与去抖（watch.go）

```go
w := shader.NewWatcher(readSource, reloader, 50*time.Millisecond)
w.Notify()        // 来源变更时调用；窗口内重复调用只触发一次重载
w.Close()         // 停止监听，等待后台 goroutine 退出，无残留状态
```

- 去抖窗口内的重复改动合并为一次重载；
- 重载在单一 goroutine 中串行执行，结果与触发顺序可复现；
- 重载失败（编译错误/不兼容）记录于 `LastError()`，旧版本继续可用，
  来源修复后的下一次变更自动恢复。

## 测试

```
go test -race ./...
```

覆盖：反射正确性与可重复性、源/产物反射一致性、截断源码、绑定冲突、
接口不变（状态保留）、各类接口不兼容、编译失败、连续改动与回退、
重载与帧提交并发（两种策略）、监听去抖与异常恢复。
