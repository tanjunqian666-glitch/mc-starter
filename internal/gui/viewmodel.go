// Package gui — ViewModel module
//
// 应用状态管理 + 线程安全的 UI 刷新绑定
// ViewModel 持有所有 UI 状态，通过 EventBus 接收事件更新自身

package gui

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gege-tlph/mc-starter/internal/config"
	"github.com/gege-tlph/mc-starter/internal/model"
)

// ============================================================
// PackItemView 下拉框展示用的版本项
// ============================================================

// PackItemView 给 UI 展示的版本信息
type PackItemView struct {
	Name         string // 包名，服务端唯一标识
	DisplayName  string // 显示名
	Primary      bool   // 是否为主版本
	LatestVersion string // 服务端最新版本
}

// ============================================================
// ViewModel
// ============================================================

// ViewModel 应用状态容器
// 所有字段通过原子操作或读写锁保护，UI 绑定方法通过 synchronize 回调刷新
type ViewModel struct {
	mu sync.RWMutex

	// 依赖（外部注入）
	cfg      *config.Manager
	localCfg *model.LocalConfig
	eventBus *EventBus
	state    *StateMachine

	// 应用数据
	cfgDir       string
	serverPacks  []model.PackInfo

	// 当前选中版本的状态
	selectedPack   string
	currentVersion string // 本地版本，"" = 未安装
	latestVersion  string // 服务端最新版本

	// 进度
	progress     int    // 0–100
	progressPhase string // 当前阶段名

	// 配置刷新标记（避免重复加载）
	packsLoaded atomic.Bool
}

// NewViewModel 创建 ViewModel
func NewViewModel(cfgDir string) *ViewModel {
	return &ViewModel{
		cfgDir: cfgDir,
		cfg:    config.New(cfgDir),
		state:  NewStateMachine(),
	}
}

// ============================================================
// 初始化
// ============================================================

// Init 执行启动时加载：配置、版本列表、版本头
// 返回是否首次运行（无 ServerURL）
func (vm *ViewModel) Init() (isFirstRun bool) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	localCfg, err := vm.cfg.LoadLocal()
	if err != nil {
		// 加载失败也返回一个空配置
		vm.localCfg = &model.LocalConfig{
			Packs: make(map[string]model.PackState),
			MinecraftDirs: map[string]string{},
		}
		return true
	}
	vm.localCfg = localCfg

	if localCfg.Packs == nil {
		localCfg.Packs = make(map[string]model.PackState)
	}
	if localCfg.MinecraftDirs == nil {
		localCfg.MinecraftDirs = map[string]string{}
	}

	isFirstRun = localCfg.ServerURL == ""

	// 加载服务端版本列表
	vm.refreshPacksLocked()

	return isFirstRun
}

// SetEventBus 注入 EventBus（在 Init 之后）
func (vm *ViewModel) SetEventBus(eb *EventBus) {
	vm.eventBus = eb

	// 状态机变更通知 EventBus
	vm.state.OnChange(func(from, to AppState) {
		if eb != nil {
			eb.EmitStateChange(from, to)
		}
	})
}

// StateMachine 返回状态机（外部可读状态，不可直接操作）
func (vm *ViewModel) StateMachine() *StateMachine {
	return vm.state
}

// LocalConfig 返回当前本地配置（只读副本）
func (vm *ViewModel) LocalConfig() *model.LocalConfig {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.localCfg.Copy()
}

// SaveLocalConfig 保存本地配置并更新内部引用
func (vm *ViewModel) SaveLocalConfig(localCfg *model.LocalConfig) error {
	if err := vm.cfg.SaveLocal(localCfg); err != nil {
		return err
	}
	vm.mu.Lock()
	vm.localCfg = localCfg
	vm.mu.Unlock()
	return nil
}

// ============================================================
// 版本列表
// ============================================================

// ServerPacks 返回服务端版本列表（只读快照）
func (vm *ViewModel) ServerPacks() []model.PackInfo {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	out := make([]model.PackInfo, len(vm.serverPacks))
	copy(out, vm.serverPacks)
	return out
}

// RefreshPacks 重新拉取服务端版本列表（协程安全）
func (vm *ViewModel) RefreshPacks() {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	vm.refreshPacksLocked()
}

func (vm *ViewModel) refreshPacksLocked() {
	if vm.localCfg == nil || vm.localCfg.ServerURL == "" {
		return
	}
	resp, err := vm.cfg.FetchPacks(vm.localCfg.ServerURL)
	if err != nil {
		return
	}
	vm.serverPacks = resp.Packs
	vm.packsLoaded.Store(true)
}

// ============================================================
// 版本选择
// ============================================================

// SelectedPack 返回当前选中的包名
func (vm *ViewModel) SelectedPack() string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.selectedPack
}

// SelectPack 切换选中版本
// 返回 (displayName, currentVersion, latestVersion)
func (vm *ViewModel) SelectPack(packName string) (displayName, currentVer, latestVer string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	vm.selectedPack = packName
	vm.updatePackStateLocked()

	displayName = vm.getDisplayNameLocked(packName)
	return displayName, vm.currentVersion, vm.latestVersion
}

// DetermineInitialPack 选择初始版本（主版本优先）
func (vm *ViewModel) DetermineInitialPack() string {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if len(vm.serverPacks) == 0 {
		return ""
	}
	for _, p := range vm.serverPacks {
		if p.Primary {
			vm.selectedPack = p.Name
			vm.latestVersion = p.LatestVersion
			vm.updatePackStateLocked()
			return p.Name
		}
	}
	vm.selectedPack = vm.serverPacks[0].Name
	vm.latestVersion = vm.serverPacks[0].LatestVersion
	vm.updatePackStateLocked()
	return vm.selectedPack
}

// ============================================================
// 版本状态查询
// ============================================================

// PackStatus 当前选中版本的完整状态
type PackStatus struct {
	PackName       string // 包名
	DisplayName    string // 显示名
	CurrentVersion string // 本地版本（"" 表示未安装）
	LatestVersion  string // 服务端最新版本
	HasUpdate      bool   // 是否有可用更新
	IsInstalled    bool   // 是否已安装
	UpdateBtnText  string // 更新按钮文案
	UpdateEnabled  bool   // 更新按钮是否可用
	StatusText     string // 状态栏文字
	StatusColor    StatusColor // 状态栏颜色
}

// StatusColor 状态颜色
type StatusColor struct {
	R, G, B byte
}

var (
	StatusGreen   = StatusColor{R: 0, G: 170, B: 0}
	StatusOrange  = StatusColor{R: 200, G: 150, B: 0}
	StatusRed     = StatusColor{R: 200, G: 50, B: 50}
	StatusGray    = StatusColor{R: 100, G: 100, B: 100}
)

// CurrentPackStatus 计算当前选中版本的显示状态
func (vm *ViewModel) CurrentPackStatus() PackStatus {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	return vm.packStatusLocked()
}

func (vm *ViewModel) packStatusLocked() PackStatus {
	s := PackStatus{
		PackName:      vm.selectedPack,
		DisplayName:   vm.getDisplayNameLocked(vm.selectedPack),
		CurrentVersion: vm.currentVersion,
		LatestVersion: vm.latestVersion,
	}

	// 无版本列表：禁用操作
	hasPacks := vm.packsLoaded.Load() && len(vm.serverPacks) > 0
	if !hasPacks {
		s.CurrentVersion = "(未安装)"
		s.IsInstalled = false
		s.HasUpdate = false
		s.UpdateBtnText = "📥 安装"
		s.UpdateEnabled = false
		s.StatusText = "未安装，请点击安装"
		s.StatusColor = StatusOrange
		return s
	}

	if vm.currentVersion == "" || vm.currentVersion == "(未安装)" {
		s.CurrentVersion = "(未安装)"
		s.IsInstalled = false
		s.HasUpdate = false
		s.UpdateBtnText = "📥 安装"
		s.UpdateEnabled = !vm.state.IsBusy()
		s.StatusText = "未安装，请点击安装"
		s.StatusColor = StatusOrange
	} else if vm.latestVersion != "" && vm.currentVersion != vm.latestVersion {
		s.IsInstalled = true
		s.HasUpdate = true
		s.UpdateBtnText = "🔄 更新"
		s.UpdateEnabled = !vm.state.IsBusy()
		s.StatusText = "有可用更新"
		s.StatusColor = StatusOrange
	} else {
		s.IsInstalled = true
		s.HasUpdate = false
		s.UpdateBtnText = "✅ 已最新"
		s.UpdateEnabled = false
		s.StatusText = "已是最新"
		s.StatusColor = StatusGreen
	}

	return s
}

// PackNames 下拉框显示名列表
func (vm *ViewModel) PackNames() []string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	names := make([]string, 0, len(vm.serverPacks))
	for _, p := range vm.serverPacks {
		display := p.DisplayName
		if vm.localCfg != nil {
			if s, ok := vm.localCfg.Packs[p.Name]; ok && s.LocalVersion != "" {
				display = fmt.Sprintf("%s v%s", p.DisplayName, s.LocalVersion)
			}
		}
		names = append(names, display)
	}
	if len(names) == 0 {
		names = append(names, "(无可用版本)")
	}
	return names
}

// VersionBarText 版本信息栏文本
func (vm *ViewModel) VersionBarText() string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	if vm.selectedPack == "" {
		return ""
	}

	cv := vm.currentVersion
	if cv == "" {
		cv = "(未安装)"
	}
	lv := vm.latestVersion
	if lv == "" {
		lv = "-"
	}

	return fmt.Sprintf("%s  本地: %s  最新: %s",
		vm.getDisplayNameLocked(vm.selectedPack), cv, lv)
}

// ============================================================
// 进度
// ============================================================

// Progress 返回当前进度
func (vm *ViewModel) Progress() (percent int, phase string) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.progress, vm.progressPhase
}

// SetProgress 设置进度（通常在 EventBus 回调中调用）
func (vm *ViewModel) SetProgress(percent int, phase string) {
	vm.mu.Lock()
	vm.progress = percent
	vm.progressPhase = phase
	vm.mu.Unlock()
}

// ============================================================
// 同步状态
// ============================================================

// CanSync 返回是否可以开始同步
func (vm *ViewModel) CanSync() bool {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	if vm.selectedPack == "" {
		return false
	}
	if vm.state.IsBusy() {
		return false
	}
	// 已安装且已最新则无更新可做
	cv := vm.currentVersion
	lv := vm.latestVersion
	if cv != "" && cv != "(未安装)" && cv == lv {
		return false
	}
	return true
}

// SyncType 返回本次同步类型
func (vm *ViewModel) SyncType() string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	if vm.currentVersion == "" || vm.currentVersion == "(未安装)" {
		return "install"
	}
	return "update"
}

// MarkSyncStart 标记同步开始（修改状态机 + 返回当前状态的快照）
func (vm *ViewModel) MarkSyncStart() {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	vm.state.Transition(StateChecking)
}

// MarkSyncDone 标记同步完成
// 自动穿透中间状态（Checking → Downloading → Installing → Done）
func (vm *ViewModel) MarkSyncDone(newVersion string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// 从当前状态穿透到 Done
	vm.pierceToLocked(StateDone)

	vm.currentVersion = newVersion

	// 写回本地配置
	if vm.localCfg != nil && vm.selectedPack != "" {
		if s, ok := vm.localCfg.Packs[vm.selectedPack]; ok {
			s.LocalVersion = newVersion
			vm.localCfg.Packs[vm.selectedPack] = s
		} else {
			vm.localCfg.Packs[vm.selectedPack] = model.PackState{
				Enabled:      true,
				Status:       "synced",
				LocalVersion: newVersion,
				Dir:          fmt.Sprintf("packs/%s", vm.selectedPack),
			}
		}
	}
}

// MarkSyncError 标记同步出错（从任何 busy 状态穿透）
func (vm *ViewModel) MarkSyncError() {
	vm.mu.Lock()
	vm.pierceToLocked(StateError)
	vm.mu.Unlock()
}

// MarkSyncCancelled 标记同步取消（从任何 busy 状态穿透）
func (vm *ViewModel) MarkSyncCancelled() {
	vm.mu.Lock()
	vm.pierceToLocked(StateCancelled)
	vm.mu.Unlock()
}

// MarkIdle 回到空闲状态（强制 Reset）
func (vm *ViewModel) MarkIdle() {
	vm.mu.Lock()
	vm.state.Reset()
	vm.mu.Unlock()
}

// pierceToLocked 从当前状态依次穿透到目标状态（调用方持锁）
// 用于跳过中间非关键状态快速到达 Error / Cancelled / Done
func (vm *ViewModel) pierceToLocked(target AppState) {
	current := vm.state.Current()
	if current == target {
		return
	}

	// 尝试直接转换
	if vm.state.Transition(target) {
		return
	}

	// 尝试穿透 Busy → Done
	if target == StateDone {
		for _, s := range []AppState{StateDownloading, StateInstalling} {
			vm.state.Transition(s)
		}
		vm.state.Transition(StateDone)
		return
	}

	// 其他目标：Reset 再转
	vm.state.Reset()
	vm.state.Transition(target)
}

// ============================================================
// 内部辅助
// ============================================================

func (vm *ViewModel) getDisplayNameLocked(packName string) string {
	for _, p := range vm.serverPacks {
		if p.Name == packName {
			return p.DisplayName
		}
	}
	return packName
}

func (vm *ViewModel) updatePackStateLocked() {
	// 读取本地版本
	if vm.localCfg != nil {
		if s, ok := vm.localCfg.Packs[vm.selectedPack]; ok {
			vm.currentVersion = s.LocalVersion
		} else {
			vm.currentVersion = ""
		}
	} else {
		vm.currentVersion = ""
	}

	// 找服务端版本
	for _, p := range vm.serverPacks {
		if p.Name == vm.selectedPack {
			vm.latestVersion = p.LatestVersion
			return
		}
	}
	vm.latestVersion = ""
}
