# Astro 博客模板个性化指南

欢迎使用本 Astro 博客模板！为了让这个博客完全变成你自己的专属空间，请按照以下步骤进行个性化配置和内容替换。

## 1. 基础信息配置

网站的基础信息主要集中在两个文件中：

*   **`astro.config.mjs`**: 
    找到 `site: "https://example.com"`，将其替换为你自己的实际域名。这是生成 Sitemap 和 RSS 订阅的必要步骤。
*   **`src/data/profile.ts`**:
    这是最重要的个人信息配置文件。你需要在这里更新：
    *   `name`: 你的昵称或名字
    *   `bio`: 一段简短的个人简介
    *   `avatar`: 你的头像链接（建议将图片放在 `public/` 目录下，然后使用相对路径，或者填入外部图床链接）
    *   `links`: 你的社交媒体链接（如 GitHub, Twitter, 邮箱等）

## 2. 页面与组件定制

*   **首页 (Hero 区域)**:
    打开 `src/components/hero.tsx` 文件。你可以修改打字机特效的文本（`phrases` 数组），以及 Hero 区域的副标题和引言，让它符合你的个人风格。
*   **关于页面**:
    打开 `src/components/pages/AboutPage.tsx` 文件。找到并替换 `<section className="mb-16">` 中的个人履历段落，写下你想向读者展示的关于你自己的故事。如果你需要接受赞赏，请更新带有二维码图片的部分。
*   **底部信息 (Footer)**:
    如果有需要备案，请前往 `src/components/footer.tsx` 底部添加你自己的 ICP 备案号。

## 3. 内容创作模块

*   **博客文章 (`src/content/posts/`)**:
    所有的长篇博客文章都存放在这里。你可以参考 `hello-world.mdx` 的 frontmatter（文章属性，如 title, date, category 等）来创建新的文章。
*   **短文/想法 (`src/content/thoughts/`)**:
    这里适合用来记录零碎的思考、日常动态或者短篇笔记。
*   **书架记录 (`public/data/books.json`)**:
    书架的数据是一个 JSON 文件。你可以根据示例的格式，添加你读过的书籍。包含书名、作者、简介、推荐状态等字段。

## 4. 社交互动

*   **友情链接 (`src/data/friends.json`)**:
    如果你有朋友的博客，可以将他们的网站信息添加到这个 JSON 文件中，格式请参考文件内的示例。
*   **评论系统**:
    本模板默认提供了一个 `Comments.astro` 占位组件。你可以轻松接入 Giscus、Waline 或者 Twikoo 等第三方评论系统。

## 5. 网站发布

在本地完成预览（`pnpm dev`）后，你可以通过 `pnpm build` 命令生成静态文件。这些文件将输出到 `dist` 目录，可以直接部署到 Vercel、Cloudflare Pages、GitHub Pages 或你自己的服务器上。

祝你写作愉快！
