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

登录 `/agenda/` 后，可在“设置 > AI 整理”中覆盖 `AI_BASE_URL`。覆盖值保存在 organizer SQLite 数据库中并立即生效；API Key 和模型仍只从服务器环境变量读取。使用“恢复服务器默认值”可重新采用 `AI_BASE_URL`。
