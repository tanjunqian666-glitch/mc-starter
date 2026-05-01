package gui

import (
	"testing"
	"time"
)

// ============================================================
// EventBus 测试
// ============================================================

func TestEventBus_EmitSubscribe(t *testing.T) {
	eb := NewEventBus(16)
	go eb.Run()
	defer eb.Close()

	ch := eb.Subscribe()

	eb.Emit(Event{Type: EvtProgress, Data: ProgressData{Percent: 50, Phase: "测试"}})

	select {
	case evt := <-ch:
		if evt.Type != EvtProgress {
			t.Fatalf("期望 EvtProgress, 得到 %v", evt.Type)
		}
		pd, ok := evt.Data.(ProgressData)
		if !ok {
			t.Fatalf("ProgressData 类型断言失败")
		}
		if pd.Percent != 50 || pd.Phase != "测试" {
			t.Fatalf("期望 (50, 测试), 得到 (%d, %s)", pd.Percent, pd.Phase)
		}
	case <-time.After(time.Second):
		t.Fatal("收事件超时")
	}

	eb.Unsubscribe(ch)
}

func TestEventBus_EmitProgress(t *testing.T) {
	eb := NewEventBus(16)
	go eb.Run()
	defer eb.Close()

	ch := eb.Subscribe()
	eb.EmitProgress(80, "下载中")

	select {
	case evt := <-ch:
		if evt.Type != EvtProgress {
			t.Fatalf("期望 EvtProgress, 得到 %v", evt.Type)
		}
		pd := evt.Data.(ProgressData)
		if pd.Percent != 80 || pd.Phase != "下载中" {
			t.Fatalf("期望 (80, 下载中), 得到 (%d, %s)", pd.Percent, pd.Phase)
		}
	case <-time.After(time.Second):
		t.Fatal("收事件超时")
	}

	eb.Unsubscribe(ch)
}

func TestEventBus_EmitLog(t *testing.T) {
	eb := NewEventBus(16)
	go eb.Run()
	defer eb.Close()

	ch := eb.Subscribe()
	eb.EmitLog("info", "日志测试")

	select {
	case evt := <-ch:
		if evt.Type != EvtLog {
			t.Fatalf("期望 EvtLog, 得到 %v", evt.Type)
		}
		ld := evt.Data.(LogData)
		if ld.Level != "info" || ld.Message != "日志测试" {
			t.Fatalf("期望 (info, 日志测试), 得到 (%s, %s)", ld.Level, ld.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("收事件超时")
	}

	eb.Unsubscribe(ch)
}

func TestEventBus_EmitError(t *testing.T) {
	eb := NewEventBus(16)
	go eb.Run()
	defer eb.Close()

	ch := eb.Subscribe()
	eb.EmitError("下载", "连接失败", nil)

	select {
	case evt := <-ch:
		if evt.Type != EvtError {
			t.Fatalf("期望 EvtError, 得到 %v", evt.Type)
		}
		ed := evt.Data.(ErrorData)
		if ed.Phase != "下载" || ed.Message != "连接失败" {
			t.Fatalf("期望 (下载, 连接失败), 得到 (%s, %s)", ed.Phase, ed.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("收事件超时")
	}

	eb.Unsubscribe(ch)
}

func TestEventBus_EmitStateChange(t *testing.T) {
	eb := NewEventBus(16)
	go eb.Run()
	defer eb.Close()

	ch := eb.Subscribe()
	eb.EmitStateChange(StateIdle, StateChecking)

	select {
	case evt := <-ch:
		if evt.Type != EvtStateChange {
			t.Fatalf("期望 EvtStateChange, 得到 %v", evt.Type)
		}
		sd := evt.Data.(StateChangeData)
		if sd.From != StateIdle || sd.To != StateChecking {
			t.Fatalf("期望 (Idle, Checking), 得到 (%v, %v)", sd.From, sd.To)
		}
	case <-time.After(time.Second):
		t.Fatal("收事件超时")
	}

	eb.Unsubscribe(ch)
}

func TestEventBus_EmitSyncDone(t *testing.T) {
	eb := NewEventBus(16)
	go eb.Run()
	defer eb.Close()

	ch := eb.Subscribe()
	eb.EmitSyncDone("主整合包", "v2.0", "v1.0", true, nil)

	select {
	case evt := <-ch:
		if evt.Type != EvtSyncDone {
			t.Fatalf("期望 EvtSyncDone, 得到 %v", evt.Type)
		}
		sd := evt.Data.(SyncDoneData)
		if sd.PackName != "主整合包" || sd.NewVersion != "v2.0" || sd.OldVersion != "v1.0" {
			t.Fatalf("数据不匹配")
		}
		if !sd.WasFullSync || sd.Err != nil {
			t.Fatalf("WasFullSync 应为 true, Err 应为 nil")
		}
	case <-time.After(time.Second):
		t.Fatal("收事件超时")
	}

	eb.Unsubscribe(ch)
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	eb := NewEventBus(32)
	go eb.Run()
	defer eb.Close()

	ch1 := eb.Subscribe()
	ch2 := eb.Subscribe()

	eb.EmitProgress(100, "完成")

	for i, ch := range []chan Event{ch1, ch2} {
		select {
		case evt := <-ch:
			if evt.Type != EvtProgress || evt.Data.(ProgressData).Percent != 100 {
				t.Fatalf("订阅者 %d 收到错误数据", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("订阅者 %d 超时", i)
		}
	}

	eb.Unsubscribe(ch1)
	eb.Unsubscribe(ch2)
}

func TestEventBus_Unsubscribe(t *testing.T) {
	eb := NewEventBus(16)
	go eb.Run()
	defer eb.Close()

	ch := eb.Subscribe()

	// 先发一个事件确认正常
	eb.EmitProgress(10, "之前")
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("Subscribe 后应收事件")
	}

	eb.Unsubscribe(ch)

	// Unsubscribe 返回后，订阅者 channel 已被关闭
	_, ok := <-ch
	if ok {
		t.Fatal("Unsubscribe 后 channel 应关闭")
	}

	// Unsubscribe 后 Emit 不应 panic
	eb.EmitProgress(50, "之后")
}

func TestEventBus_Close(t *testing.T) {
	eb := NewEventBus(16)
	go eb.Run()

	ch := eb.Subscribe()
	eb.Close()

	// Close 返回后，订阅者 channel 应被关闭
	_, ok := <-ch
	if ok {
		t.Fatal("Close 后 channel 应关闭")
	}

	// 重复 Close 不应 panic
	eb.Close()
}

func TestEventBus_BufferDrop(t *testing.T) {
	eb := NewEventBus(1) // 极小 buffer
	go eb.Run()
	defer eb.Close()

	ch := eb.Subscribe()

	// 发大量事件，buffer 满了应丢弃
	for i := 0; i < 100; i++ {
		eb.EmitProgress(i, "测试")
	}

	// 至少能收到一些（但不一定全部）
	received := 0
	for {
		select {
		case <-ch:
			received++
		case <-time.After(100 * time.Millisecond):
			goto done
		}
	}
done:
	if received == 0 {
		t.Fatal("应至少收到一些事件")
	}
}

// ============================================================
// StateMachine 测试
// ============================================================

func TestStateMachine_InitialState(t *testing.T) {
	sm := NewStateMachine()
	if sm.Current() != StateIdle {
		t.Fatalf("初始状态应为 Idle, 得到 %v", sm.Current())
	}
}

func TestStateMachine_Transition(t *testing.T) {
	sm := NewStateMachine()

	tests := []struct {
		from AppState
		to   AppState
		want bool
	}{
		{StateIdle, StateChecking, true},
		{StateChecking, StateDownloading, true},
		{StateDownloading, StateInstalling, true},
		{StateInstalling, StateDone, true},
		{StateIdle, StateError, true},
		{StateError, StateIdle, true},
		{StateIdle, StateCancelled, true},
		{StateCancelled, StateIdle, true},
		{StateDone, StateError, true},
		{StateDone, StateIdle, true},
		{StateIdle, StateRepairing, true},
		{StateRepairing, StateDone, true},
		{StateRepairing, StateError, true},
		{StateRepairing, StateCancelled, true},
	}

	for _, tt := range tests {
		sm.Reset()
		sm.current = tt.from // 绕过 Transition 直接设
		got := sm.Transition(tt.to)
		if got != tt.want {
			t.Errorf("从 %v -> %v: 期望 %v, 得到 %v", tt.from, tt.to, tt.want, got)
		}
	}
}

func TestStateMachine_InvalidTransitions(t *testing.T) {
	sm := NewStateMachine()
	sm.current = StateIdle

	// 从 Idle -> 这些应失败
	pairs := []struct {
		from AppState
		to   AppState
	}{
		{StateIdle, StateInstalling},
		{StateIdle, StateDownloading},
		{StateIdle, StateDone},
		{StateChecking, StateIdle},
		{StateChecking, StateDone},
		{StateDownloading, StateIdle},
		{StateDownloading, StateDone},
		{StateInstalling, StateIdle},
		{StateInstalling, StateDownloading},
		{StateDone, StateChecking},
		{StateDone, StateDownloading},
	}

	for _, p := range pairs {
		sm.Reset()
		sm.current = p.from
		if sm.Transition(p.to) {
			t.Errorf("从 %v -> %v 应被拒绝", p.from, p.to)
		}
	}
}

func TestStateMachine_OnChangeCallback(t *testing.T) {
	sm := NewStateMachine()

	called := false
	var capturedFrom, capturedTo AppState
	sm.OnChange(func(from, to AppState) {
		called = true
		capturedFrom = from
		capturedTo = to
	})

	sm.Transition(StateChecking)

	if !called {
		t.Fatal("OnChange 未被调用")
	}
	if capturedFrom != StateIdle || capturedTo != StateChecking {
		t.Fatalf("期望 (Idle, Checking), 得到 (%v, %v)", capturedFrom, capturedTo)
	}
}

func TestStateMachine_IsBusy(t *testing.T) {
	sm := NewStateMachine()

	if sm.IsBusy() {
		t.Fatal("Idle 不应是 busy")
	}

	sm.Transition(StateChecking)
	if !sm.IsBusy() {
		t.Fatal("Checking 应是 busy")
	}

	sm.Reset()
	if sm.IsBusy() {
		t.Fatal("Reset 后不应是 busy")
	}
}

func TestStateMachine_Reset(t *testing.T) {
	sm := NewStateMachine()
	sm.Transition(StateChecking)
	sm.Reset()

	if sm.Current() != StateIdle {
		t.Fatalf("Reset 后应为 Idle, 得到 %v", sm.Current())
	}
}

func TestStateMachine_String(t *testing.T) {
	tests := []struct {
		state AppState
		want  string
	}{
		{StateIdle, "就绪"},
		{StateChecking, "正在检查版本..."},
		{StateDownloading, "正在下载文件..."},
		{StateInstalling, "正在安装加载器..."},
		{StateDone, "完成"},
		{StateError, "出错"},
		{StateCancelled, "已取消"},
		{StateRepairing, "正在修复..."},
	}

	for _, tt := range tests {
		got := tt.state.String()
		if got != tt.want {
			t.Errorf("%v: 期望 %s, 得到 %s", tt.state, tt.want, got)
		}
	}
}
