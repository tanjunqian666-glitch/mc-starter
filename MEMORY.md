# 🦐 虾虾的长时记忆

建立：2026-04-26 · 末次优化：2026-04-30

## 配置

- **模型栈**：DeepSeek Chat（主力）→ Sonnet 4.6（备用）→ Opus 4.7（复杂推理）· 中转 `api.lingshuai.cc/v1` · Brave Search key 存环境变量
- **MemorySearch**：`bge-small-zh-v1.5`（本地中转 `127.0.0.1:8765`，中文优化）
- **Gateway**：端口 18789 loopback + token · openclaw 2026.3.13 · compact：default + memoryFlush(30k 中文 prompt) · contextPruning: cache-ttl 4h keepLastAssistants 3 · maxConcurrent 4 subagents 4

## 硬性规则

1. **勿 sudo** — 大模型幻觉可能改系统，用户帐户能搞定的就在用户帐户搞。
2. **勿猜幻觉** — 图片/语音识别后执行命令，拿不准就问鸽鸽确认，不许猜。
3. **改配置/重启先问** — gateway 重启、配置写入、自身服务变更，必须先问鸽鸽。
4. **省 token = 省事** — 高精度场景（代码/数字）用模型看或本地工具；低精度（表情/大意）本地工具秒出；执行必须精确，拿不准就问。
5. **聊天可容忍** — 聊天场景可以看大概；执行场景必须精确确认。

## 技能体系

### 核心
- **agent-browser** — Chromium 无头 · ref定位 + 可访问性树 · 反检测需设 `AGENT_BROWSER_USER_AGENT` + `AGENT_BROWSER_ARGS`(含 `--disable-blink-features=AutomationControlled`)，**启动 daemon 前设好**
- **docker-essentials** — BuildKit cache mount 实测二次构建提速 3.4×
- **ocr-local** — Tesseract.js 本地中英文 OCR，DeepSeek 不支持视觉时降级
- **voice-recognition** — Whisper 本地识别，tiny 防爆内存
- **ontology** — 类型化知识图谱 CLI
- **self-improving** — 执行误差分级存储
- **proactivity** — 主动推进框架
- **user-permissions** — 操作审批

### 🔥 妙想金融（14 技能，2026-04-27 装，基于东方财富）
详见 `mx-guide.md`。

| 层级 | 技能 | 要点 |
|------|------|------|
| **数据** | `mx-finance-data` | 行情/财务/估值 · 出 xlsx · 单次 ≤5 实体 |
| | `mx-finance-search` | 公告/研报/新闻/政策 · 出 txt |
| | `mx-macro-data` | GDP/CPI/PMI 等宏观 · 出 CSV · 注意完整性复核 |
| | `mx-stocks-screener` | 自然语言选股/基金/板块 · 出 CSV · `--select-type` 必填 |
| **问答** | `mx-financial-assistant` | 全能查数+分析+选股+百科 · `--deep-think` 可用 |
| **诊断** | `stock-diagnosis` | 个股综合诊断（仅 A 股）· Markdown 透传 |
| | `fund-diagnosis` | 单只基金诊断 · Markdown 透传 |
| | `stock-market-hotspot-discovery` | 市场热点 · query 宜简单（如"今天A股市场热点"） |
| **报告** | `industry-research-report` | 行业深度 · PDF+DOCX+分享链接 |
| | `industry-stock-tracker` | 行业/个股跟踪 · 日报/周报/月报 |
| | `initiation-of-coverage-or-deep-dive` | 首次覆盖/个股深度 · A港美北交所 |
| | `stock-earnings-review` | 业绩点评 · 3步：实体识别→报告期匹配→生成 |
| | `topic-research-report` | 专题报告 · 政策/事件/主题 |
| | `comparable-company-analysis` | 可比公司分析 · 仅 A 股 · 出 Excel |

## 运维

- **磁盘**：79G 用 14G(19%) · **内存**：3.8G，gateway ~600M，Firefox 最吃内存可关
- **常用**：`openclaw gateway restart` 先问鸽鸽 · 测试沙箱 `docker run -d --name <name> <image> sleep 3600`

## 人

- **ZIQIN**：额外聊天对象（Discord），mc-starter 项目合作者
- 技术向用户，偏好直接可执行方案

## 活跃项目: mc-starter (2026-04-30)
- **仓库**: github.com/gege-tlph/mc-starter
- **本地**: /home/claw/mc-starter（Docker golang:1.23 编译测试）
- **进度**: P1 ✅, P2.1/P2.2 ✅, P2.6/P2.7/P2.8 ✅

### Sprint 4 (Fabric + 修复栈) 已推进
| ID | 任务 | 状态 |
|----|------|------|
| P2.1 | Fabric 安装器 | ✅ |
| P2.2 | Fabric libraries 组装 | ✅ |
| P2.6 | 备份系统 | ✅ `internal/repair/backup.go` |
| P2.7 | 修复命令 | ✅ `internal/repair/repair.go` |
| P2.8 | 崩溃检测器 | ✅ `internal/repair/detector.go` (fsnotify+轮询降级) |
| P2.9 | 静默守护 | 📋 |
| P2.12 | TUI 修复界面 | 📋 |
| P2.13 | 托盘入口 | 📋 |
| P2.14 | 弹窗兜底 | 📋 |
| P2.15 | PCL2 刷新 | 📋 |

### 测试
- `internal/repair`: 31 个测试 0.15s 全过
- `internal/launcher`: ~90 测试 0.1s 全过
- `internal/pack` / `internal/mirror` / `archive/`: 全部通过
- 全项目 12 包 0 失败
