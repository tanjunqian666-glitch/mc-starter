package gui

import (
	"testing"
	"time"
)

// ============================================================
// ViewModel 测试
// ============================================================

func TestViewModel_InitFirstRun(t *testing.T) {
	vm := NewViewModel(t.TempDir())
	isFirst := vm.Init()
	if !isFirst {
		t.Fatal("空目录应是首次运行")
	}
	if vm.state.Current() != StateIdle {
		t.Fatal("初始状态应为 Idle")
	}
}

func TestViewModel_StateMachineWired(t *testing.T) {
	vm := NewViewModel(t.TempDir())
	vm.Init()

	sm := vm.StateMachine()
	if sm.Current() != StateIdle {
		t.Fatal("初始应为 Idle")
	}

	vm.MarkSyncStart()
	if sm.Current() != StateChecking {
		t.Fatal("MarkSyncStart 后应为 Checking")
	}

	vm.MarkSyncDone("v2.0")
	if sm.Current() != StateDone {
		t.Fatal("MarkSyncDone 后应为 Done")
	}

	vm.MarkIdle()
	if sm.Current() != StateIdle {
		t.Fatal("MarkIdle 后应为 Idle")
	}
}

func TestViewModel_CanSync(t *testing.T) {
	vm := NewViewModel(t.TempDir())
	vm.Init()

	if vm.CanSync() {
		t.Fatal("无版本列表时应不可同步")
	}
}

func TestViewModel_EventBusIntegration(t *testing.T) {
	vm := NewViewModel(t.TempDir())
	vm.Init()

	eb := NewEventBus(16)
	go eb.Run()

	vm.SetEventBus(eb)
	ch := eb.Subscribe()

	// Sanity: verify EventBus subscription works
	sanityCh := eb.Subscribe()
	sanityGot := make(chan struct{})
	go func() {
		<-sanityCh
		close(sanityGot)
	}()
	eb.EmitProgress(1, "sanity")
	select {
	case <-sanityGot:
	case <-time.After(time.Second):
		t.Fatal("EventBus sanity check failed: subscriber did not receive event")
	}
	eb.Unsubscribe(sanityCh)

	// Drain ch of any stale events
loop:
	for {
		select {
		case <-ch:
		default:
			break loop
		}
	}

	vm.MarkSyncStart()

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
		t.Fatal("应收 EventBus 事件")
	}

	eb.Unsubscribe(ch)
	eb.Close()
}

func TestViewModel_PackStatus(t *testing.T) {
	vm := NewViewModel(t.TempDir())
	vm.Init()

	status := vm.CurrentPackStatus()
	// 无选中版本时，应默认显示"📥 安装"
	if status.UpdateBtnText != "📥 安装" {
		t.Fatalf("无选中版本时按钮应为 📥 安装, 得到 %q", status.UpdateBtnText)
	}
	if status.UpdateEnabled {
		t.Fatal("无版本列表时按钮不应可用")
	}
	if status.StatusText != "未安装，请点击安装" {
		t.Fatalf("状态文本不对: %q", status.StatusText)
	}
}

func TestViewModel_SetProgress(t *testing.T) {
	vm := NewViewModel(t.TempDir())
	vm.Init()

	vm.SetProgress(50, "下载中")
	p, phase := vm.Progress()
	if p != 50 || phase != "下载中" {
		t.Fatalf("期望 (50, 下载中), 得到 (%d, %s)", p, phase)
	}
}

func TestViewModel_MarkSyncAndIdle(t *testing.T) {
	vm := NewViewModel(t.TempDir())
	vm.Init()

	vm.MarkSyncStart()
	vm.MarkSyncDone("v1.0")
	vm.MarkIdle()

	if vm.StateMachine().Current() != StateIdle {
		t.Fatal("最终应为 Idle")
	}
}

func TestViewModel_ErrorFlow(t *testing.T) {
	vm := NewViewModel(t.TempDir())
	vm.Init()

	vm.MarkSyncStart()
	vm.MarkSyncError()
	vm.MarkIdle()

	if vm.StateMachine().Current() != StateIdle {
		t.Fatal("错误恢复后应为 Idle")
	}
}

func TestViewModel_CancelledFlow(t *testing.T) {
	vm := NewViewModel(t.TempDir())
	vm.Init()

	vm.MarkSyncStart()
	vm.MarkSyncCancelled()
	vm.MarkIdle()

	if vm.StateMachine().Current() != StateIdle {
		t.Fatal("取消恢复后应为 Idle")
	}
}
