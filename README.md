# MC-Starter

> Lightweight Minecraft version manager + modpack updater.

No built-in launcher, no proxy bundled, no bloat. One job: **download and configure the right Minecraft version + modpack, so PCL2 (or your launcher of choice) just works.**

## Quick Start (30 seconds)

```
1. Drop starter.exe next to PCL2.exe
2. Drop config/server.json next to it
3. Double-click starter.exe
   └→ Auto-discovers PCL2 → downloads MC → installs Fabric → syncs mods → configures PCL.ini → launches PCL2
4. Click Play in PCL2 → start gaming
```

**It's a single double-click.**

If auto-detection can't find PCL2 or .minecraft, the program will prompt you to select the folders manually.

### Prerequisites

- **Java 17+** (if missing, `starter run` guides you through installation)

## Commands

| Command | Description |
|---|---|
| `starter` / `starter run` | Full auto mode: detect → sync → integrate → launch PCL2 |
| `starter run --headless` | Silent mode, no interaction |
| `starter init` | Initialize local configuration |
| `starter check` | Check Java / PCL2 / config integrity |
| `starter sync` | Sync version + mods only (no launcher launch) |
| `starter pcl detect` | Auto-detect PCL2.exe location |
| `starter pcl path <path>` | Manually set PCL2 path |
| `starter version` | Show version info |
| `starter self-update` | Update the starter itself |

### Common Flags

| Flag | Description |
|---|---|
| `--config ./my-config` | Config directory (default: `./config`) |
| `--verbose` | Verbose logging |
| `--headless` | Silent mode, no prompts |
| `--dry-run` | Check only, no download |

## Project Structure

```
mc-starter/
├── cmd/
│   └── starter/          ← entry point
├── internal/
│   ├── config/           ← config read/write
│   ├── downloader/       ← HTTP download + SHA256 verify
│   ├── logger/           ← leveled logging
│   ├── mirror/           ← BMCLAPI mirror acceleration
│   └── model/            ← shared types
├── pkg/                  ← public reusable packages
├── docs/
│   ├── zh/               ← 中文文档
│   └── ...               ← more docs
├── scripts/              ← build & dev scripts
├── Makefile
├── go.mod
└── README.md
```

## Config

### server.json (auto-updated by server, do not edit manually)
### local.json (you can edit this)

Example `local.json`:

```json
{
  "install_path": "./.minecraft",
  "launcher": "bare",
  "java_home": "",
  "memory": 4096,
  "username": "Player",
  "mirror_mode": "auto"
}
```

## Build from Source

```bash
git clone https://github.com/tanjunqian666-glitch/mc-starter.git
cd mc-starter
make build         # build for current platform
make build-all     # cross-compile: windows / linux / mac
```

## FAQ

**Q: Does this need admin privileges?**
No, unless .minecraft is under Program Files.

**Q: Does it support Forge?**
Yes. Set `loader.type` to `"forge"` in server.json.

**Q: Downloads are slow.**
Built-in Chinese mirror acceleration. Set `mirror_mode: "china"` in local.json.

**Q: Can I play multiple modpacks?**
Yes. Each modpack in its own directory with its own starter + config.

## License

MIT

---

> [中文文档 →](docs/zh/README.md)
