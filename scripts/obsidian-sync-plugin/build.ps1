# VermilionVoid Sync Plugin Build Script

# 1. Install dependencies
npm install

# 2. Build the plugin
npm run build

# 3. Refresh the ready-to-copy plugin bundle tracked in this repository
$BundleDir = Join-Path $PSScriptRoot "vermililon-void-sync"
New-Item -ItemType Directory -Force -Path $BundleDir | Out-Null
Copy-Item (Join-Path $PSScriptRoot "main.js") (Join-Path $BundleDir "main.js") -Force
Copy-Item (Join-Path $PSScriptRoot "manifest.json") (Join-Path $BundleDir "manifest.json") -Force
Copy-Item (Join-Path $PSScriptRoot "styles.css") (Join-Path $BundleDir "styles.css") -Force

Write-Host "`nBuild complete. To install the plugin manually:" -ForegroundColor Green
Write-Host "1. Create a folder in your Obsidian vault: .obsidian/plugins/vermilion-void-sync/"
Write-Host "2. Copy the contents of 'vermililon-void-sync/' to that folder."
Write-Host "3. Reload Obsidian and enable the plugin in 'Community Plugins'."
