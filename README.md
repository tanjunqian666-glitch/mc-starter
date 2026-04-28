# MC-Starter

> A small, fast Windows updater for Minecraft: download the right version + modpack, and PCL2 just works.

No launcher, no proxy, no bloat. One job.

## Quick Start

```
1. Drop starter.exe next to PCL2.exe
2. Drop config/server.json next to it
3. Double-click starter.exe
   └→ auto: find PCL2 → download MC → install Fabric → sync mods → update PCL.ini → launch PCL2
4. Click Play
```

**Just double-click and go.**

### Prerequisites

- Windows 10/11
- Java 17+ (guided install if missing)

## Commands

| Command | Description |
|---|---|
| `starter run` | Full auto: detect → sync → integrate → launch PCL2 |
| `starter init` | Initialize local config |
| `starter check` | Check Java / PCL2 / config integrity |
| `starter sync` | Sync version + mods only |
| `starter repair` | Interactive repair tool |
| `starter pcl detect` | Find PCL2.exe |
| `starter pcl path <path>` | Set PCL2 path manually |
| `starter version` | Show version |

### Flags

`--config ./dir` config dir (default `./config`)
`--verbose` / `--headless` / `--dry-run`

## Project Structure

```
mc-starter/
├── cmd/starter/       entry point
├── internal/          private packages
│   ├── config/        JSON config read/write
│   ├── downloader/    HTTP download + SHA256 verify
│   ├── logger/        leveled logging
│   ├── mirror/        BMCLAPI mirror support
│   └── model/         shared types
├── pkg/               reusable packages
├── docs/zh/           documentation (Chinese)
├── scripts/           build scripts
├── Makefile
├── go.mod
└── README.md
```

## Build

```bash
make build          # → build/starter.exe
make build-release  # GUI mode, no console window, UPX stripped
make size           # check binary size
```

## Design Goals

- **Tiny binary**: `-ldflags="-s -w"`, optional UPX, zero-fat deps
- **Low memory**: no busy loops, no polling, no hidden browser engine
- **Fast startup**: ~5ms from launch to deciding what to do
- **Windows only**: tray icon + native MessageBox, no cross-platform overhead

## License

MIT

---

> [中文文档 →](docs/zh/README.md) | [design docs →](docs/zh/)
