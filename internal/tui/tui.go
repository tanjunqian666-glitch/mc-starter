package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ============================================================
// TUI 模型 — 终端界面用于显示更新器状态
// ============================================================

// StepID 步骤标识
type StepID int

const (
	StepDetectLauncher StepID = iota // 检测启动器
	StepReadConfig                   // 读取配置
	StepFindVersion                  // 查找本地版本
	StepSyncFiles                    // 同步文件
	StepLaunch                       // 启动游戏
	StepDone                         // 完成
	StepError                        // 错误
)

// StepStatus 步骤状态
type StepStatus int

const (
	StatusPending StepStatus = iota // 等待中
	StatusRunning                   // 进行中
	StatusDone                      // 已完成
	StatusError                     // 出错
)

// FileProgress 单个文件的进度
type FileProgress struct {
	Name   string
	Pct    float64
	Status string // status icon/emoji
	Detail string
}

// SyncPhase 同步阶段
type SyncPhase struct {
	Name  string
	Total int
	Done  int
	Files []FileProgress
}

// Model TUI 状态模型
type Model struct {
	// 各步骤状态
	Steps      []StepStatus
	StepLabels []string
	StepMsgs   []string

	// 版本信息
	TargetVersion string
	VersionFound  bool
	VersionFrom   string

	// 同步进度
	SyncPhase *SyncPhase

	// 杂项
	ErrorMsg  string
	StartTime time.Time
	Quitting  bool
	Complete  bool
	ShowHelp  bool

	// 组件
	spinner   spinner.Model
	progress  progress.Model
	width     int
	height    int
}

// NewModel 创建初始模型
func NewModel() Model {
	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	s.Spinner = spinner.Dot

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	return Model{
		Steps:      make([]StepStatus, 6),
		StepLabels: []string{"检测启动器", "读取配置", "查找本地版本", "同步文件", "启动游戏", "完成"},
		StepMsgs:   make([]string, 6),
		StartTime:  time.Now(),
		spinner:    s,
		progress:   p,
	}
}

// Init tea.Model 接口
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		tea.Tick(time.Millisecond*200, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}),
	)
}

// tickMsg 定时刷新消息
type tickMsg time.Time

// Update tea.Model 接口
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = min(msg.Width-10, 40)
		if m.progress.Width < 10 {
			m.progress.Width = 10
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.Quitting = true
			return m, tea.Quit
		case "h":
			m.ShowHelp = !m.ShowHelp
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		var cmd tea.Cmd
		var pm tea.Model
		pm, cmd = m.progress.Update(msg)
		if p, ok := pm.(progress.Model); ok {
			m.progress = p
		}
		return m, cmd

	// --- 业务流程消息 ---
	case StepStartMsg:
		if int(msg.Step) < len(m.Steps) {
			m.Steps[msg.Step] = StatusRunning
			m.StepMsgs[msg.Step] = msg.Msg
		}
		return m, nil

	case StepDoneMsg:
		if int(msg.Step) < len(m.Steps) {
			m.Steps[msg.Step] = StatusDone
			m.StepMsgs[msg.Step] = msg.Msg
		}
		return m, nil

	case StepUpdateMsg:
		if int(msg.Step) < len(m.Steps) && m.Steps[msg.Step] == StatusRunning {
			m.StepMsgs[msg.Step] = msg.Msg
		}
		return m, nil

	case SyncInitMsg:
		m.SyncPhase = &SyncPhase{Name: "正在同步", Total: msg.Total}
		for _, name := range msg.Files {
			m.SyncPhase.Files = append(m.SyncPhase.Files, FileProgress{Name: name, Status: "pending"})
		}
		return m, nil

	case SyncProgressMsg:
		if m.SyncPhase != nil {
			m.SyncPhase.Done = msg.Done
			m.SyncPhase.Total = msg.Total
		}
		return m, nil

	case SyncFileMsg:
		if m.SyncPhase != nil {
			found := false
			for i := range m.SyncPhase.Files {
				if m.SyncPhase.Files[i].Name == msg.Name {
					m.SyncPhase.Files[i].Status = msg.Status
					m.SyncPhase.Files[i].Detail = msg.Detail
					found = true
					break
				}
			}
			if !found {
				m.SyncPhase.Files = append(m.SyncPhase.Files, FileProgress{Name: msg.Name, Status: msg.Status, Detail: msg.Detail})
			}
		}
		return m, nil

	case CompleteMsg:
		m.Complete = true
		m.Steps[StepDone] = StatusDone
		return m, tea.Quit
	}

	return m, nil
}

// View tea.Model 接口
func (m Model) View() string {
	if m.Quitting {
		return "\n  再见！\n"
	}

	var b strings.Builder

	// 标题栏
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Render(" MC Starter — 整合包更新器 ")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", min(m.width, 60)))
	b.WriteString("\n\n")

	// 步骤列表
	for i, label := range m.StepLabels {
		status := m.Steps[i]
		msg := m.StepMsgs[i]

		var prefix string
		switch status {
		case StatusPending:
			prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("○")
		case StatusRunning:
			prefix = m.spinner.View()
		case StatusDone:
			prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓")
		case StatusError:
			prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✗")
		}

		style := lipgloss.NewStyle()
		if status == StatusPending {
			style = style.Foreground(lipgloss.Color("240"))
		} else if status == StatusRunning {
			style = style.Bold(true)
		} else if status == StatusError {
			style = style.Foreground(lipgloss.Color("196"))
		}

		// 如果该步骤已完成或正在进行，显示标签
		stepText := fmt.Sprintf("%s %s", prefix, label)
		if i == int(StepError) && m.ErrorMsg != "" {
			stepText = fmt.Sprintf("%s %s: %s", prefix, label, m.ErrorMsg)
		}
		b.WriteString(style.Render(stepText))

		if msg != "" && (status == StatusDone || status == StatusRunning) {
			detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Italic(true)
			b.WriteString("\n  " + detailStyle.Render(msg))
		}
		b.WriteString("\n")

		// 同步阶段中显示详细的文件进度
		if i == int(StepSyncFiles) && status == StatusRunning && m.SyncPhase != nil {
			b.WriteString(m.renderSyncPhase())
		}

		b.WriteString("\n")
	}

	// 底部信息
	if m.Complete {
		completeStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("42"))
		b.WriteString(completeStyle.Render("✓ 完成！请在 3 秒后关闭...\n"))
	} else if !m.Quitting {
		hint := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("按 q 退出  ·  按 h 显示帮助")
		b.WriteString(hint)
		b.WriteString("\n")
	}

	return b.String()
}

// renderSyncPhase 渲染同步阶段的详细进度
func (m Model) renderSyncPhase() string {
	if m.SyncPhase == nil {
		return ""
	}

	var b strings.Builder
	sp := m.SyncPhase

	// 总体进度条
	pct := 0.0
	if sp.Total > 0 {
		pct = float64(sp.Done) / float64(sp.Total)
	}
	bar := m.progress.ViewAs(pct)
	info := fmt.Sprintf("%s  %d/%d 文件", bar, sp.Done, sp.Total)
	b.WriteString("    " + info + "\n")

	// 各子项进度
	for _, f := range sp.Files {
		icon := " "
		if f.Status == "done" {
			icon = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓")
		} else if f.Status == "running" {
			icon = m.spinner.View()
		}
		line := fmt.Sprintf("      %s %s", icon, f.Name)
		if f.Detail != "" {
			line += "  " + f.Detail
		}
		b.WriteString(line + "\n")
	}

	return b.String()
}

// ============================================================
// 命令辅助函数 — 更新 TUI 状态
// ============================================================

// SetStepStatus 设置步骤状态和消息
func SetStepStatus(m *Model, step StepID, status StepStatus, msg string) {
	if int(step) < len(m.Steps) {
		m.Steps[step] = status
		m.StepMsgs[step] = msg
	}
}

// SetVersionInfo 设置版本信息
func SetVersionInfo(m *Model, version string, found bool, from string) {
	m.TargetVersion = version
	m.VersionFound = found
	m.VersionFrom = from
}

// SetSyncProgress 设置同步进度
func SetSyncProgress(m *Model, name string, done, total int) {
	if m.SyncPhase == nil {
		m.SyncPhase = &SyncPhase{Name: name}
	}
	m.SyncPhase.Name = name
	m.SyncPhase.Done = done
	m.SyncPhase.Total = total
}

// AddSyncFile 添加文件进度项
func AddSyncFile(m *Model, name string) {
	if m.SyncPhase == nil {
		m.SyncPhase = &SyncPhase{}
	}
	m.SyncPhase.Files = append(m.SyncPhase.Files, FileProgress{Name: name, Status: "pending"})
}

// UpdateSyncFile 更新文件状态
func UpdateSyncFile(m *Model, name, status, detail string) {
	if m.SyncPhase == nil {
		return
	}
	for i := range m.SyncPhase.Files {
		if m.SyncPhase.Files[i].Name == name {
			m.SyncPhase.Files[i].Status = status
			m.SyncPhase.Files[i].Detail = detail
			return
		}
	}
	// 没找到就追加
	m.SyncPhase.Files = append(m.SyncPhase.Files, FileProgress{Name: name, Status: status, Detail: detail})
}

// SetError 设置错误信息
func SetError(m *Model, msg string) {
	m.ErrorMsg = msg
	m.Steps[StepError] = StatusError
}

// SetComplete 标记完成
func SetComplete(m *Model) {
	m.Complete = true
	m.Steps[StepDone] = StatusDone
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
