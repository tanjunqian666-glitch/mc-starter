// Package gui — EventBus module
//
// 模块间事件传递：进度、日志、错误、状态变更
// 纯 channel 实现，无外部依赖

package gui

// ============================================================
// 事件类型
// ============================================================

// EventType 事件分类
type EventType int

const (
	EvtProgress EventType = iota
	EvtLog
	EvtError
	EvtStateChange
	EvtPackList      // 服务端版本列表刷新完成
	EvtSyncDone      // 同步完成
	EvtSyncCancelled // 用户取消同步
)

// Event 统一事件结构
type Event struct {
	Type EventType
	Data interface{}
}

// ============================================================
// 事件数据结构
// ============================================================

// ProgressData 进度变更
type ProgressData struct {
	Percent int    // 0–100
	Phase   string // "检查更新" / "下载文件" / "安装 Fabric" / "安装 Forge" / ...
}

// LogData 日志消息
type LogData struct {
	Level   string // "info" / "warn" / "error"
	Message string
}

// ErrorData 错误信息
type ErrorData struct {
	Phase   string // 出错的阶段
	Message string // 错误描述
	Err     error  // 原始 error（可为 nil）
}

// StateChangeData 状态机状态变更
type StateChangeData struct {
	From AppState
	To   AppState
}

// SyncDoneData 同步完成
type SyncDoneData struct {
	PackName    string // 更新的版本包名
	NewVersion  string // 更新后的版本号
	OldVersion  string // 更新前的版本号
	WasFullSync bool   // 是否为全量同步（非增量）
	Err         error  // 出错了就填这个
}

// ============================================================
// EventBus
// ============================================================

// EventBus 模块间事件总线
// 使用带缓冲的 channel 防止阻塞发送方
// 单消费者设计：主协程订阅，其他协程 emit
type EventBus struct {
	ch      chan Event
	sub     chan subReq
	unsub   chan unsubReq
	closed  chan struct{}
	done    chan struct{} // Run 退出后关闭
}

type subReq struct {
	ch chan Event
	ok chan struct{} // 注册完成后通知
}

type unsubReq struct {
	ch chan Event
	ok chan struct{} // unsub 完成后通知
}

// NewEventBus 创建一个新的事件总线，bufferSize 为事件通道缓冲大小
func NewEventBus(bufferSize int) *EventBus {
	if bufferSize <= 0 {
		bufferSize = 64
	}
	return &EventBus{
		ch:     make(chan Event, bufferSize),
		sub:    make(chan subReq, 1),
		unsub:  make(chan unsubReq, 1),
		closed: make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Subscribe 注册一个消费者 channel，返回该 channel
// 阻塞直到注册完成，确保后续 Emit 的事件能被收到
// 调用方应循环读取 ch，使用 range 或 select
func (eb *EventBus) Subscribe() chan Event {
	ch := make(chan Event, 64)
	req := subReq{ch: ch, ok: make(chan struct{})}
	eb.sub <- req
	<-req.ok
	return ch
}

// Unsubscribe 移除指定的消费者 channel，并等待移除完成
func (eb *EventBus) Unsubscribe(ch chan Event) {
	req := unsubReq{ch: ch, ok: make(chan struct{})}
	eb.unsub <- req
	<-req.ok
}

// Emit 发送一个事件（非阻塞：超过 buffer 时丢弃）
func (eb *EventBus) Emit(evt Event) {
	select {
	case eb.ch <- evt:
	default:
	}
}

// EmitProgress 快捷方法：发送进度事件
func (eb *EventBus) EmitProgress(percent int, phase string) {
	eb.Emit(Event{Type: EvtProgress, Data: ProgressData{Percent: percent, Phase: phase}})
}

// EmitLog 快捷方法：发送日志事件
func (eb *EventBus) EmitLog(level, message string) {
	eb.Emit(Event{Type: EvtLog, Data: LogData{Level: level, Message: message}})
}

// EmitError 快捷方法：发送错误事件
func (eb *EventBus) EmitError(phase, message string, err error) {
	eb.Emit(Event{Type: EvtError, Data: ErrorData{Phase: phase, Message: message, Err: err}})
}

// EmitStateChange 快捷方法：发送状态变更事件
func (eb *EventBus) EmitStateChange(from, to AppState) {
	eb.Emit(Event{Type: EvtStateChange, Data: StateChangeData{From: from, To: to}})
}

// EmitSyncDone 快捷方法：发送同步完成事件
func (eb *EventBus) EmitSyncDone(packName, newVersion, oldVersion string, wasFull bool, err error) {
	eb.Emit(Event{Type: EvtSyncDone, Data: SyncDoneData{
		PackName:    packName,
		NewVersion:  newVersion,
		OldVersion:  oldVersion,
		WasFullSync: wasFull,
		Err:         err,
	}})
}

// Run 启动事件分发循环，在单独的 goroutine 中运行
// 将事件广播给所有已订阅的消费者
func (eb *EventBus) Run() {
	subs := make(map[chan Event]struct{})

	defer close(eb.done)

	for {
		select {
		case evt := <-eb.ch:
			for ch := range subs {
				select {
				case ch <- evt:
				default:
				}
			}

		case req := <-eb.sub:
			subs[req.ch] = struct{}{}
			close(req.ok)

		case req := <-eb.unsub:
			// 先 drain 这个订阅者的 channel 防止它被 close 后写入 deadlock
			//（理论上 Emit 不再发给它后不会有新数据，但 buffer 里已有的会残留）
			delete(subs, req.ch)
			close(req.ch)
			close(req.ok)

		case <-eb.closed:
			for ch := range subs {
				close(ch)
			}
			return
		}
	}
}

// Close 关闭事件总线，停止分发并关闭所有订阅 channel
func (eb *EventBus) Close() {
	select {
	case <-eb.closed:
		return
	default:
		close(eb.closed)
	}
	<-eb.done // 等待 Run 退出
}
