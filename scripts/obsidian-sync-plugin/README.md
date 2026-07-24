# VermilionVoid Sync 0.2

Obsidian 内置的博客文章审核与发布管理器。

## 安装

1. 在本目录运行 `npm install` 和 `npm run build`。
2. 将 `main.js`、`manifest.json`、`styles.css` 复制到 Vault 的 `.obsidian/plugins/vermilion-void-sync/`。
3. 在 Obsidian 中启用插件，并配置同步服务地址、Webhook Secret 和本地文章目录。

## 工作流

1. 打开“VermilionVoid 文章管理”。
2. 点击“获取并处理文章”。服务器只生成待审核任务，不会自动发布。
3. 编辑元数据和正文，审批 AI 提出的新标签，然后保存到本地。
4. 勾选文章并点击“发布所选”，服务器将批准版本推送到 `deploy`。

处理期间如果本地文件发生变化，插件会拒绝覆盖并要求重新创建任务。
