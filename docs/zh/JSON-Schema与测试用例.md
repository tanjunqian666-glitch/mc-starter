# JSON Schema 与测试用例

> 旧版 server.json schema 已废弃 → 改用 REST API 响应格式
> 保留 local.json schema

---

## local.json

```json
{
  "minecraft_dir": "/home/user/.minecraft",
  "server_url": "https://mc.example.com:8443",
  "server_token": "",
  "packs": {
    "main-pack": {
      "enabled": true,
      "status": "synced",
      "local_version": "v1.2.0",
      "dir": "packs/main-pack"
    }
  },
  "launcher": "auto",
  "memory": 4096
}
```

## REST API 增量响应（测试示例）

```json
{
  "version": "v1.2.0",
  "from_version": "v1.1.0",
  "mode": "incremental",
  "added": [
    {"path": "mods/iris.jar", "hash": "a1b2c3d4e5f6abcdef1234567890abcdef1234567890abcdef1234567890abcd", "size": 4096000}
  ],
  "updated": [],
  "removed": ["mods/optifine.jar"],
  "total_diff_bytes": 4096000
}
```

## 验证规则

| 字段 | 必须 | 类型 | 限制 |
|------|------|------|------|
| version | ✅ | string | 非空 |
| from_version | ❌ | string | 首次发布可为空 |
| mode | ✅ | string | `incremental` 或 `full` |
| added[].hash | ✅ | string | 64 字符 SHA256 hex |
| added[].size | ✅ | int | > 0 |
| removed[] | ❌ | string[] | 可为空数组 |
