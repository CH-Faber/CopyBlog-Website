/**
 * 站点级配置。使用本模板时请先修改此处与 `src/data/profile.ts`。
 * `siteUrl` 需与 `astro.config` 中 `site` 保持一致（由本文件导出并在 `astro.config.ts` 中引用）。
 */
export const siteUrl = "https://example.com"

export const siteName = "My Blog"

/** 页脚版权行显示名称 */
export const copyrightName = "Your Name"

/**
 * ICP 备案号（仅展示文案）。不需要备案时设为 `null`，页脚将不显示备案链接。
 */
export const icpNumber: string | null = null

/** 主题仓库链接（页脚「主题：…」） */
export const themeRepoUrl = "https://github.com/Lapis0x0/VermilionVoid"

export function absoluteSiteUrl(path: string): string {
  const base = siteUrl.replace(/\/$/, "")
  if (path.startsWith("http://") || path.startsWith("https://")) return path
  const normalized = path.startsWith("/") ? path : `/${path}`
  return `${base}${normalized}`
}
