import { getCollection, type CollectionEntry } from "astro:content"
import MarkdownIt from "markdown-it"

export type ThoughtEntry = CollectionEntry<"thoughts">

const md = new MarkdownIt()

const stripHtml = (html: string) => {
  const withBreaks = html
    .replace(/<\s*br\s*\/?>/gi, "\n")
    .replace(/<\/p>/gi, "\n")
    .replace(/<\/li>/gi, "\n")
  const text = withBreaks.replace(/<[^>]*>/g, "")
  return text.replace(/\n\s*\n+/g, "\n").trim()
}

const truncateText = (text: string, maxLength: number) => {
  if (text.length <= maxLength) return text
  const slice = text.slice(0, Math.max(0, maxLength)).replace(/\s+$/g, "")
  return `${slice}...`
}

const extractImages = (html: string) => {
  const images: { src: string; alt: string }[] = []
  const imagePattern = /<img\b[^>]*>/gi
  const srcPattern = /\bsrc=["']([^"']+)["']/i
  const altPattern = /\balt=["']([^"']*)["']/i

  for (const match of html.matchAll(imagePattern)) {
    const element = match[0]
    const src = element.match(srcPattern)?.[1]
    if (!src) continue
    images.push({ src, alt: element.match(altPattern)?.[1] ?? "" })
  }

  return {
    images,
    content: html.replace(imagePattern, ""),
  }
}

export async function getSortedThoughts(): Promise<ThoughtEntry[]> {
  const allThoughts = await getCollection("thoughts")

  const sorted = allThoughts.sort((a: ThoughtEntry, b: ThoughtEntry) => {
    const dateA = new Date(a.data.published)
    const dateB = new Date(b.data.published)
    return dateA > dateB ? -1 : 1
  })

  return sorted
}

export const toThoughtMeta = (thought: ThoughtEntry) => {
  const tags = thought.data.tags?.length ? thought.data.tags : []
  const body = typeof thought.body === "string" ? thought.body : ""
  const rendered = extractImages(md.render(body))

  return {
    slug: thought.slug,
    title: thought.data.title,
    content: rendered.content,
    plainText: stripHtml(rendered.content),
    images: rendered.images,
    date: thought.data.published.toISOString(),
    tags,
  }
}

export const toThoughtPreview = (thought: ThoughtEntry, maxLength = 96) => {
  const body = typeof thought.body === "string" ? thought.body : ""
  const rendered = extractImages(md.render(body))
  const text = stripHtml(rendered.content)

  return {
    slug: thought.slug,
    title: thought.data.title,
    date: thought.data.published.toISOString(),
    excerpt: truncateText(text, maxLength),
    image: rendered.images[0]?.src,
  }
}

export const buildThoughtsTags = (thoughts: { tags: string[] }[]) => {
  return Array.from(new Set(thoughts.flatMap((t) => t.tags || []))).sort((a, b) =>
    a.localeCompare(b),
  )
}
