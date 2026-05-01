// Package gui — State Machine module
//
// AppState 定义和状态转换规则
// type int + switch 实现，无外部依赖

package gui

// ============================================================
// 状态定义
// ============================================================

// AppState GUI 应用状态
type AppState int

const (
	StateIdle AppState = iota
	StateChecking         // 检查版本
	StateDownloading      // 下载文件
	StateInstalling       // 安装 Fabric/Forge
	StateDone             // 完成
	StateError            // 出错
	StateCancelled        // 用户取消
	StateRepairing        // 修复中
)

// String 返回状态的可读名称，用于 UI 显示
func (s AppState) String() string {
	switch s {
	case StateIdle:
		return "就绪"
	case StateChecking:
		return "正在检查版本..."
	case StateDownloading:
		return "正在下载文件..."
	case StateInstalling:
		return "正在安装加载器..."
	case StateDone:
		return "完成"
	case StateError:
		return "出错"
	case StateCancelled:
		return "已取消"
	case StateRepairing:
		return "正在修复..."
	default:
		return "未知"
	}
}

// ============================================================
// 状态转换规则
//
// 允许的转换：
//   Idle ──→ Checking ──→ Downloading ──→ Installing ──→ Done
//    │          │               │                │
//    └────→ Error ←─────────────┴────────────────┘
//    │
//    └────→ Cancelled
//
//                     Idle ──→ Repairing ──→ Done
//                       │                    │
//                       └────→ Error ←───────┘
//                       │
//                       └────→ Cancelled
// ============================================================

// validTransitions 记录每个状态允许跳转到的目标状态集合
var validTransitions = map[AppState]map[AppState]bool{
	StateIdle: {
		StateChecking:  true,
		StateRepairing: true,
		StateError:     true,
		StateCancelled: true,
	},
	StateChecking: {
		StateDownloading: true,
		StateError:       true,
		StateCancelled:   true,
	},
	StateDownloading: {
		StateInstalling: true,
		StateError:      true,
		StateCancelled:  true,
	},
	StateInstalling: {
		StateDone: true,
		StateError: true,
		StateCancelled: true,
	},
	StateDone: {
		StateIdle: true,   // 完成后回到 Idle 准备下次操作
		StateError: true,
	},
	StateError: {
		StateIdle: true,   // 错误后允许重试
	},
	StateCancelled: {
		StateIdle: true,   // 取消后允许重试
	},
	StateRepairing: {
		StateDone:  true,
		StateError: true,
		StateCancelled: true,
	},
}

// ============================================================
// StateMachine
// ============================================================

// StateMachine 状态机，管理 AppState 的合法转换
type StateMachine struct {
	current AppState
	onChange func(from, to AppState) // 状态变更回调（可选）
}

// NewStateMachine 创建状态机，初始状态为 StateIdle
func NewStateMachine() *StateMachine {
	return &StateMachine{
		current: StateIdle,
	}
}

// Current 返回当前状态
func (sm *StateMachine) Current() AppState {
	return sm.current
}

// OnChange 设置状态变更回调
// 调用方可以在这里注入 EventBus.EmitStateChange
func (sm *StateMachine) OnChange(fn func(from, to AppState)) {
	sm.onChange = fn
}

// Transition 尝试转换到新状态
// 返回是否转换成功
func (sm *StateMachine) Transition(to AppState) bool {
	if sm.current == to {
		// 相同状态允许（幂等）
		return true
	}

	allowed, ok := validTransitions[sm.current]
	if !ok || !allowed[to] {
		return false
	}

	from := sm.current
	sm.current = to

	if sm.onChange != nil {
		sm.onChange(from, to)
	}

	return true
}

// IsBusy 返回当前是否在忙碌状态（非 Idle/Error/Done/Cancelled）
func (sm *StateMachine) IsBusy() bool {
	switch sm.current {
	case StateIdle, StateDone, StateError, StateCancelled:
		return false
	default:
		return true
	}
}

// Reset 重置到 Idle 状态（强制，不验证转换规则）
func (sm *StateMachine) Reset() {
	from := sm.current
	sm.current = StateIdle
	if sm.onChange != nil {
		sm.onChange(from, StateIdle)
	}
}
