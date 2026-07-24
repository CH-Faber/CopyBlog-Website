# 一个闪念 · 内容管理 0.5

Obsidian 内置的博客文章审核与发布管理器。

## 安装

1. 在本目录运行 `npm install` 和 `npm run build`。
2. 将 `main.js`、`manifest.json`、`styles.css` 复制到 Vault 的 `.obsidian/plugins/flash-thought-content-manager/`。
3. 在 Obsidian 中启用插件，并配置同步服务地址、Webhook Secret、本地文章目录和可选的本地 AI 服务。

## 工作流

1. 打开“一个闪念 · 内容管理”。
2. 点击“获取并处理文章”。服务器只拉取并暂存文章，不使用本地 AI 密钥，也不会自动发布。
3. 插件使用 Obsidian 本地配置的模型生成摘要、分类和标签建议；也可以点击“AI 分析当前”重新分析。
4. 编辑元数据和正文，审批 AI 提出的新分类和新标签，然后保存到本地。
5. 勾选文章并点击“发布所选”，服务器将批准版本推送到 `deploy`。

处理期间如果本地文件发生变化，插件会拒绝覆盖并要求重新创建任务。

分类与标签均在管理页维护。文章只能选择一个已启用分类；AI 提出的新分类或新标签必须批准后才能使用。

## 密钥说明

- `Webhook Secret` 用于验证文章管理和发布请求，服务器与插件两端都需要配置。
- `AI API Key` 只保存在 Vault 的插件 `data.json` 中，由 Obsidian 直接请求 OpenAI 兼容接口，不会发送给同步服务器，也不要提交到 Git。
