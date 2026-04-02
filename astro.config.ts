import { defineConfig } from "astro/config"
import react from "@astrojs/react"
import sitemap from "@astrojs/sitemap"
import { pluginCollapsibleSections } from "@expressive-code/plugin-collapsible-sections"
import { pluginLineNumbers } from "@expressive-code/plugin-line-numbers"
import expressiveCode from "astro-expressive-code"
import rehypeAutolinkHeadings from "rehype-autolink-headings"
import rehypeComponents from "rehype-components"
import rehypeKatex from "rehype-katex"
import rehypeSlug from "rehype-slug"
import remarkDirective from "remark-directive"
import remarkGithubAdmonitionsToDirectives from "remark-github-admonitions-to-directives"
import remarkMath from "remark-math"
import remarkSectionize from "remark-sectionize"
import { remarkShiftHeadings } from "./src/plugins/remark-shift-headings.mjs"
import { AdmonitionComponent } from "./src/plugins/rehype-component-admonition.mjs"
import { GithubCardComponent } from "./src/plugins/rehype-component-github-card.mjs"
import { parseDirectiveNode } from "./src/plugins/remark-directive-rehype.js"
import { remarkExcerpt } from "./src/plugins/remark-excerpt.js"
import { remarkReadingTime } from "./src/plugins/remark-reading-time.mjs"
import { pluginLanguageBadge } from "./src/plugins/expressive-code/language-badge"
import { pluginCustomCopyButton } from "./src/plugins/expressive-code/custom-copy-button"
import { siteUrl } from "./src/config/site"
import path from "node:path"
import { fileURLToPath } from "node:url"

const __dirname = path.dirname(fileURLToPath(import.meta.url))

type AdmonitionKind = "note" | "tip" | "important" | "caution" | "warning"

const createAdmonitionComponent =
  (type: AdmonitionKind) => (properties = {}, children: unknown[] = []) => {
    const normalizedChildren = Array.isArray(children) ? children : []
    return AdmonitionComponent(properties, normalizedChildren, type)
  }

export default defineConfig({
  srcDir: "./src",
  output: "static",
  site: siteUrl,
  trailingSlash: "always",
  vite: {
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
  },
  integrations: [
    expressiveCode({
      themes: ["github-dark", "github-light"],
      themeCssSelector: (theme) => {
        if (theme.name === "github-light") return ":root:not(.dark)"
        return ".dark"
      },
      plugins: [
        pluginCollapsibleSections(),
        pluginLineNumbers(),
        pluginLanguageBadge(),
        pluginCustomCopyButton(),
      ],
      defaultProps: {
        wrap: true,
        overridesByLang: {
          shellsession: {
            showLineNumbers: false,
          },
        },
      },
      styleOverrides: {
        codeBackground: "var(--codeblock-bg)",
        borderRadius: "0.75rem",
        borderColor: "transparent",
        codeFontSize: "0.875rem",
        codeFontFamily: "var(--font-mono), ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
        codeLineHeight: "1.625",
        frames: {
          editorBackground: "var(--codeblock-bg)",
          terminalBackground: "var(--codeblock-bg)",
          terminalTitlebarBackground: "var(--codeblock-topbar-bg)",
          editorTabBarBackground: "var(--codeblock-topbar-bg)",
          editorActiveTabBackground: "transparent",
          editorActiveTabIndicatorBottomColor: "var(--primary)",
          editorActiveTabIndicatorTopColor: "transparent",
          editorTabBarBorderBottomColor: "var(--codeblock-topbar-bg)",
          terminalTitlebarBorderBottomColor: "transparent",
        },
      },
      frames: {
        showCopyToClipboardButton: false,
      },
    }),
    react(),
    sitemap(),
  ],
  markdown: {
    remarkPlugins: [
      remarkMath,
      remarkReadingTime,
      remarkExcerpt,
      remarkShiftHeadings,
      remarkGithubAdmonitionsToDirectives,
      remarkDirective,
      remarkSectionize,
      parseDirectiveNode,
    ],
    rehypePlugins: [
      rehypeKatex,
      rehypeSlug,
      [
        rehypeComponents,
        {
          components: {
            github: GithubCardComponent,
            note: createAdmonitionComponent("note"),
            tip: createAdmonitionComponent("tip"),
            important: createAdmonitionComponent("important"),
            caution: createAdmonitionComponent("caution"),
            warning: createAdmonitionComponent("warning"),
          },
        },
      ],
      [
        rehypeAutolinkHeadings,
        {
          behavior: "append",
          properties: {
            className: ["anchor"],
          },
          content: {
            type: "element",
            tagName: "span",
            properties: {
              className: ["anchor-icon"],
              "data-pagefind-ignore": true,
            },
            children: [
              {
                type: "text",
                value: "#",
              },
            ],
          },
        },
      ],
    ],
  },
})
