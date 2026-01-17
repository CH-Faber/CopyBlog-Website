import { getCollection, type CollectionEntry } from "astro:content"
import MarkdownIt from "markdown-it"

export type ThoughtEntry = CollectionEntry<"thoughts">

const md = new MarkdownIt()

const formatDate = (date?: Date) => {
  if (!date) return ""
  return date.toISOString().slice(0, 10)
}

const stripHtml = (html: string) => {
  return html.replace(/<[^>]*>/g, "").replace(/\s+/g, " ").trim()
}

const truncateText = (text: string, maxLength: number) => {
  if (text.length <= maxLength) return text
  return `${text.slice(0, Math.max(0, maxLength)).trim()}...`
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
  const content = md.render(body)

  return {
    slug: thought.slug,
    title: thought.data.title,
    content,
    date: formatDate(thought.data.published),
    tags,
  }
}

export const toThoughtPreview = (thought: ThoughtEntry, maxLength = 96) => {
  const body = typeof thought.body === "string" ? thought.body : ""
  const content = md.render(body)
  const text = stripHtml(content)

  return {
    slug: thought.slug,
    title: thought.data.title,
    date: formatDate(thought.data.published),
    excerpt: truncateText(text, maxLength),
  }
}

export const buildThoughtsTags = (thoughts: { tags: string[] }[]) => {
  return Array.from(new Set(thoughts.flatMap((t) => t.tags || []))).sort((a, b) =>
    a.localeCompare(b),
  )
}
