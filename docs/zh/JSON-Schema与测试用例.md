# MC 版本更新器 — JSON Schema & 测试用例清单

---

## 一、JSON Schema

### 1.1 server.json Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Minecraft Starter Server Config",
  "description": "服务端下发配置，只读。客户端首次 sync 时从服务器获取",
  "type": "object",
  "required": ["mc_version", "modpack"],
  "properties": {
    "version_tag": {
      "type": "string",
      "description": "配置版本标识，客户端用于判断是否需要重新同步",
      "default": "stable"
    },
    "mc_version": {
      "type": "string",
      "description": "Minecraft 版本号，如 1.20.1、1.21",
      "examples": ["1.20.1", "1.21", "1.20.4"]
    },
    "loader": {
      "type": "object",
      "description": "模组加载器配置（可选，不配则不安装加载器）",
      "properties": {
        "type": {
          "type": "string",
          "enum": ["fabric", "forge"],
          "description": "加载器类型"
        },
        "version": {
          "type": "string",
          "description": "加载器版本号（可选，不指定则使用最新版）",
          "examples": ["0.15.11", "47.1.43"]
        }
      },
      "required": ["type"]
    },
    "modpack": {
      "type": "object",
      "description": "整合包配置（packwiz 格式）",
      "required": ["url"],
      "properties": {
        "url": {
          "type": "string",
          "format": "uri",
          "description": "pack.toml 的下载地址"
        },
        "hash": {
          "type": "string",
          "description": "pack.toml 的 SHA256 校验值（可选）",
          "pattern": "^sha256:[a-f0-9]{64}$"
        }
      }
    },
    "java": {
      "type": "object",
      "description": "Java 环境要求",
      "properties": {
        "min_version": {
          "type": "integer",
          "description": "最低 Java 主版本号",
          "default": 17,
          "examples": [17, 21]
        },
        "download_url": {
          "type": "string",
          "format": "uri",
          "description": "用户无 Java 时引导下载的地址"
        },
        "auto_install": {
          "type": "boolean",
          "description": "是否自动下载安装 Java",
          "default": false
        }
      }
    },
    "mirrors": {
      "type": "object",
      "description": "自定义镜像覆盖",
      "patternProperties": {
        "^[a-z_]+$": {
          "type": "string",
          "format": "uri"
        }
      }
    }
  }
}
```

### 1.2 local.json Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Minecraft Starter Local Config",
  "description": "本地用户配置，可手动编辑，由 starter init 生成",
  "type": "object",
  "properties": {
    "install_path": {
      "type": "string",
      "description": "Minecraft 安装目录",
      "default": "./.minecraft",
      "examples": ["./.minecraft", "C:\\Users\\User\\mc\\.minecraft"]
    },
    "work_dir": {
      "type": "string",
      "description": "工作目录（配置文件、缓存存放位置）",
      "default": "."
    },
    "launcher": {
      "type": "string",
      "enum": ["pcl2", "hmcl", "vanilla"],
      "description": "MC 启动器类型，pcl2/hmcl/vanilla",
      "default": "pcl2"
    },
    "java_home": {
      "type": "string",
      "description": "指定 Java 路径（留空则自动检测）",
      "default": ""
    },
    "memory": {
      "type": "integer",
      "description": "分配的内存（MB）",
      "default": 4096,
      "minimum": 1024,
      "maximum": 65536
    },
    "username": {
      "type": "string",
      "description": "离线模式用户名",
      "default": "Player",
      "minLength": 1,
      "maxLength": 16
    },
    "resolution": {
      "type": "object",
      "description": "游戏窗口分辨率",
      "properties": {
        "width": { "type": "integer", "default": 854, "minimum": 640 },
        "height": { "type": "integer", "default": 480, "minimum": 480 }
      }
    },
    "jvm_args": {
      "type": "string",
      "description": "额外 JVM 参数",
      "default": ""
    },
    "mirror_mode": {
      "type": "string",
      "enum": ["auto", "china", "global"],
      "description": "镜像模式",
      "default": "auto"
    },
    "pcl2": {
      "type": "object",
      "description": "PCL2 集成配置",
      "properties": {
        "path": {
          "type": "string",
          "description": "PCL2.exe 的路径（留空则自动检测）",
          "default": ""
        },
        "version_name": {
          "type": "string",
          "description": "在 PCL2 中显示的版本名称（版本文件夹名）",
          "default": "",
          "examples": ["main-test", "小猪之家2.0"]
        },
        "update_pcl_ini": {
          "type": "boolean",
          "description": "是否自动更新 PCL.ini（卡片、版本选择）",
          "default": true
        },
        "update_card": {
          "type": "boolean",
          "description": "是否自动在版本卡片中注册新版本",
          "default": true
        },
        "mode": {
          "type": "string",
          "enum": ["auto", "manual", "disabled"],
          "description": "PCL2 集成模式",
          "default": "auto"
        }
      }
    }
  }
}
```

### 1.3 配置校验函数（Go 实现）

```go
// 在 config/validator.go 中
import "github.com/xeipuuv/gojsonschema"

func ValidateServerConfig(data []byte) error {
    schemaLoader := gojsonschema.NewStringLoader(serverConfigSchema)
    docLoader := gojsonschema.NewBytesLoader(data)
    
    result, err := gojsonschema.Validate(schemaLoader, docLoader)
    if err != nil {
        return fmt.Errorf("schema 加载失败: %w", err)
    }
    
    if !result.Valid() {
        var errs []string
        for _, desc := range result.Errors() {
            errs = append(errs, desc.String())
        }
        return &StarterError{
            Code: 3,
            UserMsg: fmt.Sprintf("配置文件格式错误：\n%s", strings.Join(errs, "\n")),
        }
    }
    return nil
}
```

---

## 二、测试用例清单

### P0 测试

| ID | 测试内容 | 类型 | 预期 |
|---|---|---|---|
| T0.1 | `go build` 编译成功 | 构建 | 无错误，生成二进制 |
| T0.2 | `./starter --help` | CLI | 显示所有子命令和 flag |
| T0.3 | `./starter init` | CLI | 生成默认 local.json |
| T0.4 | 配置系统：读取完整 JSON | 单元 | Config 结构体字段正确 |
| T0.5 | 配置系统：缺失字段用默认值 | 单元 | 默认值生效 |
| T0.6 | 配置系统：local.json 覆盖 server.json | 单元 | local 优先级更高 |
| T0.7 | mirror.Select：主镜像可用 | 单元 | 返回主镜像 URL |
| T0.8 | mirror.FallbackDo：主镜像失败回退 | 单元 | 切换为备用镜像 |
| T0.9 | mirror.FallbackDo：全部失败 | 单元 | 返回错误 |
| T0.10 | downloader.Download：正常下载 | 单元/集成 | 文件存在且内容正确 |
| T0.11 | downloader.Download：hash 校验失败 | 单元 | 返回 hash 错误 |
| T0.12 | downloader.Download：连接超时 | 单元 | 返回超时错误 + 可重试标记 |
| T0.13 | downloader.Download：进度回调 | 单元 | 回调被调用且数值合理 |
| T0.14 | downloader.Download：ETag 缓存命中 | 单元 | 跳过下载（304） |
| T0.15 | 日志系统：分级输出 | 单元 | DEBUG/INFO/WARN/ERROR 正确分级 |

### P1 测试

| ID | 测试内容 | 类型 | 预期 |
|---|---|---|---|
| T1.1 | 版本清单：解析正常响应 | 单元 | 版本列表正确 |
| T1.2 | 版本清单：网络失败用缓存 | 集成 | 返回缓存数据 |
| T1.3 | 版本 Jar：下载并校验 SHA1 | 集成 | 文件 + hash 匹配 |
| T1.4 | Asset 索引：解析正常 | 单元 | object 列表正确 |
| T1.5 | Asset 下载：并发下载指定文件 | 集成 | 所有文件在正确路径 |
| T1.6 | Asset 下载：文件已存在则跳过 | 集成 | 不重新下载 |
| T1.7 | Asset 下载：部分失败 | 集成 | 失败文件记录并继续 |
| T1.8 | Libraries 下载：路径生成 | 单元 | `com.example:lib:1.0` → 正确路径 |
| T1.9 | 完整 sync：从头到尾跑通 | 集成（mock） | .minecraft 结构完整 |
| T1.10 | 断点恢复：kill 后重新 sync | 集成 | 继续而非重头下载 |

### P2 测试

| ID | 测试内容 | 类型 | 预期 |
|---|---|---|---|
| T2.1 | Fabric Meta 版本列表解析 | 单元 | 列表正确 |
| T2.2 | Fabric Profile JSON 解析 | 单元 | mainClass + libraries 正确 |
| T2.3 | 启动参数：classpath 拼接 | 单元 | 包含所有必要 jar |
| T2.4 | 启动参数：JVM args 生成 | 单元 | 含 -Xmx、-Djava.library.path 等 |
| T2.5 | 启动参数：MC args 生成 | 单元 | 含 --username、--version 等 |
| T2.6 | 完整 sync+launch（mock java） | 集成 | java 被调用且有正确参数 |

### P3 测试

| ID | 测试内容 | 类型 | 预期 |
|---|---|---|---|
| T3.1 | Java 检测：JAVA_HOME 生效 | 单元 | 路径正确 |
| T3.2 | Java 检测：PATH 搜索 | 单元 | 找到 java |
| T3.3 | Java 版本解析：`java -version` 输出 | 单元 | 主版本号提取正确 |
| T3.4 | Java 版本：过低报错 | 单元 | 返回明确的升级提示 |
| T3.5 | Java 不存在报错 | 单元 | 输出下载引导 |

### P4 测试

| ID | 测试内容 | 类型 | 预期 |
|---|---|---|---|
| T4.1 | launcher_profiles.json 生成 | 单元 | 格式正确，PCL2 可读 |
| T4.2 | HMCL json 生成 | 单元 | 格式正确 |
| T4.3 | 多启动器选择 | 单元 | launcher 参数切换输出格式 |

### P5 测试

| ID | 测试内容 | 类型 | 预期 |
|---|---|---|---|
| T5.1 | 版本比较：需要更新 | 单元 | 返回 true |
| T5.2 | 版本比较：已是最新 | 单元 | 返回 false |
| T5.3 | 版本比较：版本号格式错误 | 单元 | 返回错误 |
| T5.4 | 自更新：下载 → 校验 → 替换 | 集成 | 新文件替换旧文件 |
| T5.5 | 自更新：hash 不匹配 | 集成 | 不替换，报错 |
| T5.6 | 回滚：bak 文件恢复 | 集成 | 原始文件恢复 |
| T5.7 | 多通道：stable 指向不同 server.json | 单元 | 不同通道不同源 |

---

## 三、手动测试 Checklist（Windows 实机）

在发布前必须跑通：

- [ ] 首次运行 `starter init` → 生成配置
- [ ] `starter check` → 正确检测 Java
- [ ] `starter sync` → .minecraft 完整，asset 数正确
- [ ] `starter sync`（第二次）→ 跳过已下载文件，秒完成
- [ ] `starter sync` 中途 Ctrl+C → 下次继续而非重头
- [ ] 断网后 `starter sync` → 提示网络错误 + 用缓存继续
- [ ] 指定不存在的 MC 版本 → 明确报错
- [ ] 修改 local.json memory → 启动时生效
- [ ] 自更新流程 → 下载新版本并重启
- [ ] 回滚 → starter.exe.bak 还原正常
