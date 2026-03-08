# 本地一键打包并部署到带有 Nginx 的 VPS 服务器
$ServerIP = "8.148.197.94"
$User = "root"
$TargetDir = "/var/www"
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
    Write-Host "Visit http://$ServerIP to see your site." -ForegroundColor Green
} else {
    Write-Host "`n>>> Deploy failed, files may not have been fully uploaded." -ForegroundColor Red
}
