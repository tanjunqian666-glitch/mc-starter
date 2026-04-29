# MC-Starter

> Minecraft updater. Double-click, wait, play.

## Quick Start

```
1. Put starter.exe next to PCL2.exe
2. Put config/server.json together
3. Double-click starter.exe
   └→ auto: find PCL2 → download MC → install Fabric → sync mods → patch PCL.ini → launch PCL2
4. Click Play
```

### Prerequisites

- Windows 10/11
- Java 17+ (will guide you if missing)

## Commands

| Command | Description |
|---|---|
| `starter run` | Full auto: detect → sync → integrate → launch PCL2 |
| `starter sync` | Sync MC version (jar/asset/library) + create repo snapshot |
| `starter check` | Check Java / PCL2 / config integrity |
| `starter init` | Initialize local config |
| `starter backup list \| create \| restore \| delete` | Local repo snapshot management |
| `starter cache stats \| clean \| prune` | CacheStore management |
| `starter pack import <zip>` | Server-side: import modpack zip → diff → draft |
| `starter pack publish` | Server-side: publish draft version |
| `starter pack diff <v1> <v2>` | Server-side: compare two versions |
| `starter pack list` | Server-side: list version history |
| `starter pcl detect` | Find PCL2.exe (4-layer progressive detection) |
| `starter pcl path <path>` | Set PCL2 path manually |
| `starter version` | Show version |

### Flags

`--config ./dir` config directory (default `./config`)
`--verbose` / `--headless` / `--dry-run`

## Build

```bash
make build          # → build/starter.exe
make build-release  # GUI mode, no console window
make size           # check binary size
```

## Design Goals

- **Small**: stdlib deps, `-ldflags="-s -w"`, optional UPX
- **Light**: no polling, no browser engine, no extra processes
- **Fast**: ~5ms from launch to decision
- **Focused**: Windows only, system tray + native dialogs

## License

MIT

---

> [中文文档 →](docs/zh/README.md)
