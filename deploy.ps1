# 本地一键打包并部署到 VPS（通过 SSH/SCP 覆盖静态目录）
# 使用前请设置环境变量，或在下方填入占位值：
#   $env:DEPLOY_SSH_HOST = "your.server.ip"
#   $env:DEPLOY_SSH_USER = "root"
#   $env:DEPLOY_TARGET_DIR = "/var/www/html"

$ServerIP = if ($env:DEPLOY_SSH_HOST) { $env:DEPLOY_SSH_HOST } else { "YOUR_SERVER_IP" }
$User = if ($env:DEPLOY_SSH_USER) { $env:DEPLOY_SSH_USER } else { "YOUR_SSH_USER" }
$TargetDir = if ($env:DEPLOY_TARGET_DIR) { $env:DEPLOY_TARGET_DIR } else { "/var/www/html" }
$SourceDir = "dist/"

Write-Host ">>> Step 1: Building static site (pnpm run build)..." -ForegroundColor Cyan
pnpm run build
if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed, aborting deployment!" -ForegroundColor Red
    exit 1
}

Write-Host "`n>>> Step 2: Uploading static files to $ServerIP ..." -ForegroundColor Cyan

$SshCmd = "mkdir -p $TargetDir && rm -rf ${TargetDir}/*"
ssh -o StrictHostKeyChecking=accept-new "${User}@${ServerIP}" $SshCmd

scp -o StrictHostKeyChecking=accept-new -r "${SourceDir}." "${User}@${ServerIP}:${TargetDir}"

if ($LASTEXITCODE -eq 0) {
    Write-Host "`n>>> Deploy succeeded!" -ForegroundColor Green
    Write-Host "Visit your site URL to verify." -ForegroundColor Green
} else {
    Write-Host "`n>>> Deploy failed, files may not have been fully uploaded." -ForegroundColor Red
}
