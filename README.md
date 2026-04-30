# MC-Starter

> Minecraft modpack updater. Configure once, auto-update forever.

mc-starter is a Minecraft modpack management tool with a C/S (client/server) architecture. Admins publish pack versions on the server, players get incremental updates with one click. Works alongside PCL2 / HMCL launchers.

## Quick Start

**For players:**
```
1. Download starter-gui.exe
2. First launch → setup wizard (API URL → launcher path → MC dir)
3. Select version → click Update (auto incremental sync)
4. Click Open Launcher → play
```

**For server admins:**
```bash
# 1. Generate default config
mc-starter-server init

# 2. Start the server
mc-starter-server start

# 3. Create a pack
curl -X POST http://localhost:8443/api/v1/admin/packs \
  -H "Authorization: Bearer change-me-please" \
  -d '{"name":"main-pack","display_name":"主服整合包","primary":true}'

# 4. Import zip and publish
curl -X POST http://localhost:8443/api/v1/admin/packs/main-pack/import \
  -F "file=@modpack.zip"
curl -X POST http://localhost:8443/api/v1/admin/packs/main-pack/publish
```

## Architecture

```
Admin:                          Player:
┌─────────────────────┐         ┌──────────────────────┐
│  mc-starter-server   │  REST  │  starter (CLI / GUI)  │
│  ─── REST API (8443) │◄──────►│                      │
│  ─── Multi-pack Mgr  │  API   │  ─── Incremental sync│
│  ─── Version publish │        │  ─── CacheStore       │
│  ─── File storage    │        │  ─── Fabric install   │
│  ─── Token auth      │        │  ─── Crash daemon     │
└─────────────────────┘         └──────────────────────┘
```

## GUI

```
┌─ MC Starter ──────────────────────[⚙]─┐
│                                          │
│  Pack: [Main Pack    v1.2.0       ▼]    │
│                                          │
│  [📂 Open Launcher]  [🔄 Sync Update]   │
│                                          │
│  Status: local v1.2.0 → server v1.3.0   │
│  Update available                        │
└──────────────────────────────────────────┘
```

## CLI Commands

| Command | Description |
|---|---|
| `starter update` | Incremental modpack sync (via server API) |
| `starter sync` | Sync MC version files (jar/asset/library) |
| `starter run` | Full auto: detect → sync → launch launcher |
| `starter repair` | Repair tool (clean mods/config/rollback) |
| `starter daemon` | Crash daemon (background monitor + auto-repair) |
| `starter backup` | Snapshot management (create/rollback/delete) |
| `starter cache` | Cache management |
| `starter fabric install` | Fabric installer download & assembly |
| `starter pack` | Server-side pack management (import/publish/diff) |
| `starter pcl` | PCL2 detection / path config |
| `starter init / check` | Init config / check integrity |
| `starter version` | Show version |

### Server Commands

```bash
mc-starter-server start [--config server.yml]   # Start the server
mc-starter-server init                           # Generate default config
mc-starter-server check                          # Validate config
```

## Requirements

- Windows 10/11 (GUI + full features)
- Linux (CLI + server only)
- Java 17+ (auto-guided if missing)

## Design Goals

- **Small tool**: users don't need to understand Fabric/Forge/Java args
- **C/S architecture**: admins manage versions, players get seamless updates
- **Unattended**: auto-install loaders, users only see a progress bar
- **Windows native**: walk GUI, no browser engine, no extra processes

## Building

```bash
# CLI (cross-platform)
go build -o starter ./cmd/starter/

# GUI (Windows only, needs MinGW-w64 + rsrc)
rsrc -manifest gui.manifest -o gui.syso
CGO_ENABLED=1 GOOS=windows go build -ldflags="-s -w -H windowsgui" -o starter-gui.exe ./cmd/starter/

# Server (cross-platform)
go build -o mc-starter-server ./cmd/mc-starter-server/
```

## Project Status

| Module | Status |
|--------|--------|
| P1 Version download + repo + incremental sync | ✅ Done |
| P2 Fabric install + repair + crash daemon | ✅ Done |
| P0x Server REST API + auth + deployment | ✅ Done |
| P3 Self-update (multi-channel + rollback) | ✅ Done |
| P6 Channel system (multi-channel + optional install) | ✅ Done |
| GUI (walk native Windows) | ✅ Windows tested |
| P5 Launcher awareness (detect + dir identify + integration) | ✅ Done |
| S10 Full integration: sync + loader + pack update | 🚧 Code complete, run merge pending |

## License

MIT

## Acknowledgements

- [PCL2](https://github.com/Hex-Dragon/PCL2) — Primary reference for launcher architecture. mc-starter's rule matching, crash detection strategy, and incremental update design are all derived from deep study of PCL2's source code.
- [HMCL](https://github.com/huanghongxun/HMCL) — Reference for multi-version isolation and multi-launcher compatibility.
- [MCUpdater / Grass-block](https://github.com/Grass-block/MCUpdater) — Minecraft client resource update system whose Channel design and multi-pack version tracking approach informed mc-starter's P6 channel system plans.

---

> [中文文档 →](docs/zh/README.md)
