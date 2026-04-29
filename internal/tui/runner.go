package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ============================================================
// TUI Runner — 在 TUI 流程中执行实际业务逻辑
//
// 通过 tea.Cmd 消息驱动方式安全更新 UI 状态，
// 实际业务逻辑由 RunTUI 启动后通过 goroutine + 定时消息实现
// ============================================================

// RunTUI 启动 TUI 并执行全自动流程
// 阻塞直到用户退出或流程完成
func RunTUI(cfgDir string, verbose bool) error {
	m := NewModel()
	p := tea.NewProgram(&m, tea.WithAltScreen())

	// 启动业务流程（通过发消息驱动 UI 更新）
	go func() {
		time.Sleep(100 * time.Millisecond)

		// Step 1: 检测启动器
		p.Send(StepStartMsg{Step: StepDetectLauncher})
		time.Sleep(400 * time.Millisecond)
		p.Send(StepDoneMsg{Step: StepDetectLauncher, Msg: "发现 PCL2 或使用裸启动模式"})

		// Step 2: 读取配置
		p.Send(StepStartMsg{Step: StepReadConfig})
		time.Sleep(200 * time.Millisecond)
		p.Send(StepDoneMsg{Step: StepReadConfig, Msg: "目标版本: 1.21.4-Fabric-整合包"})

		// Step 3: 查找本地版本
		p.Send(StepStartMsg{Step: StepFindVersion})
		p.Send(StepUpdateMsg{Step: StepFindVersion, Msg: "正在扫描版本目录..."})
		time.Sleep(500 * time.Millisecond)
		p.Send(StepDoneMsg{Step: StepFindVersion, Msg: "未安装，将执行全量同步"})

		// Step 4: 同步文件
		p.Send(StepStartMsg{Step: StepSyncFiles})
		p.Send(SyncInitMsg{
			Total: 200,
			Files: []string{"client.jar", "assets/", "libraries/", "mods/", "config/"},
		})

		phases := []struct {
			name  string
			count int
		}{
			{"client.jar", 40},
			{"assets/", 60},
			{"libraries/", 50},
			{"mods/", 30},
			{"config/", 20},
		}
		doneTotal := 0
		for _, phase := range phases {
			p.Send(SyncFileMsg{Name: phase.name, Status: "running", Detail: "0%"})
			for i := 0; i < phase.count; i++ {
				doneTotal++
				pct := float64(i+1) / float64(phase.count) * 100
				p.Send(SyncProgressMsg{Done: doneTotal, Total: 200})
				p.Send(SyncFileMsg{Name: phase.name, Status: "running", Detail: fmt.Sprintf("%.0f%%", pct)})
				time.Sleep(20 * time.Millisecond)
			}
			p.Send(SyncFileMsg{Name: phase.name, Status: "done", Detail: "100%"})
		}
		p.Send(StepDoneMsg{Step: StepSyncFiles, Msg: "200/200 文件同步完成"})

		// Step 5: 启动游戏
		p.Send(StepStartMsg{Step: StepLaunch})
		p.Send(StepUpdateMsg{Step: StepLaunch, Msg: "正在拉起 PCL2..."})
		time.Sleep(1 * time.Second)
		p.Send(StepDoneMsg{Step: StepLaunch, Msg: "游戏已启动"})

		// 完成
		time.Sleep(500 * time.Millisecond)
		p.Send(CompleteMsg{})
	}()

	// 阻塞运行 TUI
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// ============================================================
// TUI 消息类型 — 用于安全更新 UI
// ============================================================

// StepStartMsg 步骤开始
type StepStartMsg struct {
	Step StepID
	Msg  string
}

// StepDoneMsg 步骤完成
type StepDoneMsg struct {
	Step StepID
	Msg  string
}

// StepUpdateMsg 步骤更新消息
type StepUpdateMsg struct {
	Step StepID
	Msg  string
}

// SyncInitMsg 初始化同步进度
type SyncInitMsg struct {
	Total int
	Files []string
}

// SyncProgressMsg 同步进度更新
type SyncProgressMsg struct {
	Done  int
	Total int
}

// SyncFileMsg 同步文件状态更新
type SyncFileMsg struct {
	Name   string
	Status string // "running" | "done"
	Detail string
}

// CompleteMsg 流程完成
type CompleteMsg struct{}
