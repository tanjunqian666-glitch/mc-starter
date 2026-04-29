# API 接口文档（更新：C/S 模式 REST API）

> 本文档定义 mc-starter-server 的 HTTP REST API。
> 详见 `服务端架构与部署.md` 和 `客户端与服务端通信.md`

---

## 一、基础

- **Base URL**: `https://{server}:{port}/api/v1`
- **认证**: 管理端需 `Authorization: Bearer {token}` 头
- **格式**: 全部 JSON
- **编码**: UTF-8

## 二、客户端端点

| 端点 | 方法 | 说明 | 认证 |
|------|------|------|------|
| `/ping` | GET | 健康检查 | 否 |
| `/packs` | GET | 包列表 | 否 |
| `/packs/{name}` | GET | 包详情 | 否 |
| `/packs/{name}/update` | GET | 增量清单 | 否 |
| `/packs/{name}/files/{hash}` | GET | 文件下载 | 否 |

### GET /packs/{name}/update

Query: `?from=v1.1.0`

Response 200:
```json
{
  "version": "v1.2.0",
  "from_version": "v1.1.0",
  "mode": "incremental",
  "added": [
    {"path": "mods/iris.jar", "hash": "a1b2c3...", "size": 4096000}
  ],
  "updated": [
    {"path": "mods/sodium.jar", "hash": "d4e5f6...", "size": 5120000}
  ],
  "removed": ["mods/optifine.jar"],
  "total_diff_bytes": 9216000
}
```

或 `mode: "full"`（首次发布，无上一版本时）→ 客户端下载全量包。

## 三、管理端端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/admin/packs` | POST | 创建包 |
| `/admin/packs/{name}` | DELETE | 删除包 |
| `/admin/packs/{name}/config` | GET/PUT | 配置读写 |
| `/admin/packs/{name}/import` | POST | 上传 zip |
| `/admin/packs/{name}/publish` | POST | 发布 |
| `/admin/packs/{name}/versions` | GET | 版本历史 |
| `/admin/packs/{name}/versions/{ver}` | DELETE | 删除版本 |

### POST /admin/packs

```json
{
  "name": "main-pack",
  "display_name": "主服整合包",
  "description": "主服务器玩法资源包",
  "primary": true
}
```

### POST /admin/packs/{name}/publish

```json
{
  "message": "更新模组版本",
  "primary": true
}
```

## 四、错误响应格式

```json
{
  "error": {
    "code": "PACK_NOT_FOUND",
    "message": "整合包 'xxx' 不存在"
  }
}
```

常见错误码：

| code | HTTP | 说明 |
|------|------|------|
| `PACK_NOT_FOUND` | 404 | 包不存在 |
| `VERSION_NOT_FOUND` | 404 | 版本不存在 |
| `UNAUTHORIZED` | 401 | token 缺失/无效 |
| `FORBIDDEN` | 403 | 权限不足 |
| `DRAFT_EXISTS` | 409 | 已有未发布的 draft |
| `IMPORT_FAILED` | 400 | zip 导入失败 |
| `INTERNAL_ERROR` | 500 | 服务端错误 |
