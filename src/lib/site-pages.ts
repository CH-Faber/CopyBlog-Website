import sitePagesData from "@/data/site-pages.json"

export type SitePageInfo = {
  key: string
  name: string
  title: string
  description: string
  heading: string
  subtitle: string
}

const pages = sitePagesData.pages as SitePageInfo[]

export const getSitePage = (key: string): SitePageInfo => {
  const page = pages.find((item) => item.key === key)
  if (!page) throw new Error(`Missing site page configuration: ${key}`)
  return page
}
