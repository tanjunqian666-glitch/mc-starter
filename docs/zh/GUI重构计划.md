# GUI 重构计划

> 2026-05-01 · 基于代码现状分析 · 待 UI 设计完成后执行

---

## 一、现状问题

当前 `internal/gui/` 的核心问题：

| 问题 | 表现 | 原因 |
|------|------|------|
| UI 与业务逻辑耦合 | `app.go` 里混着 walk 控件引用、状态管理、假的更新逻辑 | 没有分层 |
| 更新流程是假的 | `startSync()` 只有进度条动画，没有调用真正的 `UpdatePack()` | GUI 与 launcher 包未对接 |
| 状态管理散乱 | `syncing bool`、`hasUpdate` 等散落在 `App` 各处，切换靠手写 if/else | 无状态机 |
| 无进度回调 | `UpdatePack()` 没有向外报告进度的接口 | 模块间无事件通道 |
| 代码集中在 app.go | 394 行包含 UI 构建、状态管理、业务逻辑、事件处理 | 没有分层 |

---

## 二、目标架构

```
┌───────────┐     ┌────────────┐     ┌──────────────┐     ┌───────────────┐
│  Walk GUI  │ ──→ │ ViewModel  │ ──→ │ Orchestrator │ ──→ │ Core Services │
│  (app.go)  │     │ (状态+绑定) │     │ (调度)       │     │ (launcher/)   │
└───────────┘     └────────────┘     └──────────────┘     └───────────────┘
                        │                    │
                        ▼                    ▼
                  EventBus ←────────── ProgressEvent
                                              LogEvent
                                              ErrorEvent
                                              StateEvent
```

### 各层职责

| 层 | 职责 | 文件位置 |
|----|------|---------|
| **GUI Layer** | Walk 控件定义、UI 布局、按钮回调转发到 ViewModel | `internal/gui/app.go` |
| **ViewModel** | 应用状态（版本号、进度、状态）、线程安全的 UI 刷新、命令绑定 | `internal/gui/viewmodel.go` |
| **Orchestrator** | Update/Repair/Launch 流程的调度逻辑，连接 launcher 包，emit 事件 | `internal/gui/orchestrator.go` |
| **EventBus** | 模块间事件传递：进度、日志、错误、状态变更 | `internal/gui/eventbus.go` |
| **StateMachine** | 状态定义和转换规则，防止 UI 状态不一致 | `internal/gui/state.go` |
| **Core Services** | 已有的 launcher 包保持不变 | `internal/launcher/` |

---

## 三、要做的改进

### 1. 事件总线（EventBus）

**必要性：** 现在 `UpdatePack()` 没有进度回调，直接调只会卡住 UI。EventBus 让下载器/安装器在 goroutine 里跑、通过事件通知 GUI 更新。

**初始事件类型：**

```go
type EventType int

const (
    EvtProgress EventType = iota
    EvtLog
    EvtError
    EvtStateChange
)

type Event struct {
    Type  EventType
    Data  interface{}
}

type ProgressData struct {
    Percent int
    Phase   string  // "检查更新" / "下载文件" / "安装Fabric"
}

type LogData struct {
    Level   string
    Message string
}
```

**用法：**

```go
// 下载器里
bus.Publish(Event{Type: EvtProgress, Data: ProgressData{Percent: 50, Phase: "下载文件"}})

// GUI 订阅
bus.Subscribe(func(e Event) {
    switch e.Type {
    case EvtProgress:
        p := e.Data.(ProgressData)
        vm.setProgress(p.Percent)
    }
})
```

### 2. 状态机（StateMachine）

**必要性：** 当前用 `syncing bool` 一个标识控制所有状态切换，出错了或者取消了 UI 状态容易卡死。

**状态定义：**

```go
type AppState int

const (
    StateIdle AppState = iota
    StateChecking          // 检查版本
    StateDownloading       // 下载文件
    StateInstalling        // 安装 Fabric/Forge
    StateDone              // 完成
    StateError             // 出错
    StateCancelled         // 用户取消
)
```

**允许的状态转换：**

```
Idle ──→ Checking ──→ Downloading ──→ Installing ──→ Done
  │                    │                 │
  └────→ Error ←───────┴─────────────────┘
  │
  └────→ Cancelled
```

**用法：**

```go
func (sm *StateMachine) Transition(to AppState) error {
    if !allowedTransitions[sm.current][to] {
        return fmt.Errorf("不允许的状态转换: %v → %v", sm.current, to)
    }
    sm.current = to
    bus.Publish(Event{Type: EvtStateChange, Data: to})
    return nil
}
```

### 3. ViewModel 层

**必要性：** 当前 `App` 结构体把 walk 控件引用和业务数据混在一起。分离 ViewModel 后：
- 业务逻辑不 import walk 包
- 数据变更自动通知 UI 刷新
- 更容易加单元测试

**初始设计：**

```go
type ViewModel struct {
    // 状态
    State     AppState
    Progress  int
    Status    string

    // 数据
    CurrentVersion string
    LatestVersion  string
    SelectedPack   string

    // 命令
    UpdateCmd func()
    RepairCmd func()
    LaunchCmd func()

    // 内部
    bus  *EventBus
    sm   *StateMachine
}
```

### 4. Orchestrator 层

**必要性：** 把"更新/修复/启动"等流程的调度逻辑从 GUI 中抽出来。

**初始设计：**

```go
type Orchestrator struct {
    cfgDir  string
    mcDir   string
    bus     *EventBus
    sm      *StateMachine
}

func (o *Orchestrator) StartUpdate(serverURL, packName string, packState *model.PackState) {
    go func() {
        o.sm.Transition(StateChecking)
        o.bus.Publish(EvtLog, "检查版本...")

        updater := launcher.NewUpdater(o.cfgDir, o.mcDir, config.New(o.cfgDir))
        result, err := updater.UpdatePack(serverURL, packName, packState, false)

        if err != nil {
            o.sm.Transition(StateError)
            o.bus.Publish(EvtError, err.Error())
            return
        }

        // ... 继续 EnsureVersion, 刷新PCL2, 保存配置
        o.sm.Transition(StateDone)
    }()
}
```

---

## 四、执行顺序

| 顺序 | 要做的事 | 涉及文件 |
|------|---------|---------|
| 1 | 建 EventBus | `internal/gui/eventbus.go` |
| 2 | 建 StateMachine | `internal/gui/state.go` |
| 3 | 从 `App` 中抽 ViewModel | `internal/gui/viewmodel.go` |
| 4 | 建 Orchestrator，接管 startSync | `internal/gui/orchestrator.go` |
| 5 | 简化 `app.go`，只留 UI 布局 | `internal/gui/app.go` |
| 6 | 真更新流程接上（UpdatePack + EnsureVersion） | `app.go` → `orchestrator.go` |
| 7 | 加修复按钮 + 修复流程 | `app.go` → `orchestrator.go` |

---

## 五、不做的事

- ❌ CLI 子进程调用（`exec.Command`）——没有意义
- ❌ 拆 Service 包（`update/`, `sync/`, `repair/`）——当前代码规模不需要
- ❌ 第三方事件总线库——一个结构体+channel 就够了
- ❌ 状态机库——`type int + switch` 已经够用

---

*本计划待 UI 设计完成后执行。当前 GUI 现状是假更新逻辑 + 无分层，重构后应达到真更新 + 可维护的分层架构。*
