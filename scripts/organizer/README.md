# Faber Organizer

独立于 gosync 的个人事项、日程、AI 整理和 Web Push 服务。生产环境只监听 `127.0.0.1:3020`，由 `faberhu.top/api/organizer/` 反向代理。

## Local development

```powershell
go mod download
$password = Read-Host -AsSecureString
# 将密码通过标准输入交给 `go run . hash-password`，把得到的 bcrypt 哈希写入本地环境变量。
go test ./...
go run .
```

不要把真实密码、AI 密钥、VAPID 私钥、会话或 SQLite 数据库提交到 Git。

## Agenda configuration

Agenda 使用 `AGENDA_*` 环境变量。原有的 `ORGANIZER_*` 名称是临时兼容别名，
用于平滑迁移已有部署；同一设置同时存在时，`AGENDA_*` 始终优先。独立部署时，
可参考仓库根目录的 `.env.example`，将数据放在 `/var/lib/agenda` 下。
