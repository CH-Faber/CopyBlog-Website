"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import type { MouseEvent } from "react"
import { createPortal } from "react-dom"
import {
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  Copy,
  Image as ImageIcon,
  Link as LinkIcon,
  X,
} from "lucide-react"
import { profile } from "@/data/profile"
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
} from "@/components/ui/pagination"
import { cn } from "@/lib/utils"

export type ThoughtMeta = {
  slug: string
  title?: string
  content: string
  plainText: string
  date: string
  tags: string[]
  images: { src: string; alt: string }[]
}

type PaginationMeta = {
  currentPage: number
  basePath?: string
  pageSize?: number
}

const HIDDEN = -1
const VISIBLE_PAGES = 5

const normalizeBasePath = (basePath: string) => {
  if (!basePath.startsWith("/")) return `/${basePath}`.replace(/\/+$/, "")
  return basePath === "/" ? "" : basePath.replace(/\/+$/, "")
}

const getPageHref = (pageNumber: number, basePath: string) => {
  const base = normalizeBasePath(basePath)
  if (pageNumber === 1) return base || "/"
  return `${base}/${pageNumber}/`
}

const buildPageRange = (currentPage: number, totalPages: number) => {
  if (totalPages <= 1) return []
  const pages: number[] = []
  const start = Math.max(1, Math.min(currentPage - 2, totalPages - 4))
  const end = Math.min(totalPages, start + VISIBLE_PAGES - 1)
  if (start > 1) pages.push(1, ...(start > 2 ? [HIDDEN] : []))
  for (let page = start; page <= end; page += 1) pages.push(page)
  if (end < totalPages) pages.push(...(end < totalPages - 1 ? [HIDDEN] : []), totalPages)
  return pages
}

const dateKey = (dateString: string) => {
  const date = new Date(dateString)
  return Number.isNaN(date.getTime()) ? dateString : date.toLocaleDateString("zh-CN")
}

const dateHeading = (dateString: string) => {
  const date = new Date(dateString)
  if (Number.isNaN(date.getTime())) return dateString
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const target = new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
  const days = Math.round((today - target) / 86400000)
  if (days === 0) return "今天"
  if (days === 1) return "昨天"
  if (date.getFullYear() === now.getFullYear()) return `${date.getMonth() + 1}月${date.getDate()}日`
  return `${date.getFullYear()}年${date.getMonth() + 1}月${date.getDate()}日`
}

const timeLabel = (dateString: string) => {
  const date = new Date(dateString)
  if (Number.isNaN(date.getTime())) return dateString
  return date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })
}

const fullDateLabel = (dateString: string) => {
  const date = new Date(dateString)
  if (Number.isNaN(date.getTime())) return dateString
  return date.toLocaleString("zh-CN", { dateStyle: "long", timeStyle: "short" })
}

function ThoughtMedia({ images, onOpen }: { images: ThoughtMeta["images"]; onOpen: (index: number) => void }) {
  if (!images.length) return null
  const visibleImages = images.slice(0, 9)
  const isSingle = images.length === 1
  const columns = images.length === 2 || images.length === 4 ? "grid-cols-2" : "grid-cols-3"
  const layout = isSingle ? "w-fit max-w-full" : cn("grid max-w-[17.5rem] grid-flow-row gap-1 sm:max-w-[20rem]", columns)

  return (
    <div className={cn("mt-4", layout)}>
      {visibleImages.map((image, index) => {
        const remaining = images.length - 9
        return (
          <button key={`${image.src}-${index}`} type="button" className={cn("group relative block overflow-hidden bg-muted/50 text-left focus:outline-none focus-visible:ring-2 focus-visible:ring-primary", isSingle ? "max-w-[68vw] rounded-lg" : "aspect-square rounded-[0.3rem]")} onClick={() => onOpen(index)} aria-label={`查看第 ${index + 1} 张图片`}>
            <img src={image.src} alt={image.alt || "闪念配图"} loading="lazy" decoding="async" className={cn("block transition-transform duration-300 group-hover:scale-[1.02]", isSingle ? "h-auto max-h-[24rem] max-w-full w-auto object-contain" : "h-full w-full object-cover")} />
            {index === 8 && remaining > 0 ? <span className="absolute inset-0 flex items-center justify-center bg-black/45 text-2xl font-semibold text-white">+{remaining}</span> : null}
          </button>
        )
      })}
    </div>
  )
}

function ThoughtCard({ thought, index }: { thought: ThoughtMeta; index: number }) {
  const [expanded, setExpanded] = useState(false)
  const [copied, setCopied] = useState(false)
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null)
  const hasMore = thought.plainText.length > 280 || (thought.content.match(/<p\b/gi)?.length ?? 0) > 4
  const href = `/thoughts/#thought-${thought.slug}`

  const copyLink = async () => {
    if (typeof window === "undefined") return
    const url = `${window.location.origin}${href}`
    try {
      await navigator.clipboard.writeText(url)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1800)
    } catch {
      window.prompt("复制这条闪念的链接：", url)
    }
  }

  useEffect(() => {
    if (lightboxIndex === null) return
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = "hidden"
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setLightboxIndex(null)
    }
    document.addEventListener("keydown", closeOnEscape)
    return () => {
      document.removeEventListener("keydown", closeOnEscape)
      document.body.style.overflow = previousOverflow
    }
  }, [lightboxIndex])

  const activeImage = lightboxIndex === null ? null : thought.images[lightboxIndex]
  const lightbox = activeImage && typeof document !== "undefined"
    ? createPortal(
        <div
          className="fixed inset-0 z-[100] flex items-center justify-center bg-black/90 p-4 backdrop-blur-sm"
          role="dialog"
          aria-modal="true"
          aria-label="图片预览"
          onClick={() => setLightboxIndex(null)}
        >
          <img
            src={activeImage.src}
            alt={activeImage.alt || "闪念配图"}
            className="max-h-[88dvh] max-w-[94vw] object-contain"
            onClick={(event) => event.stopPropagation()}
          />
          <button
            type="button"
            className="fixed right-4 top-4 z-[101] inline-flex min-h-11 items-center gap-1.5 rounded-full border border-white/20 bg-black/70 px-4 py-2 text-sm font-medium text-white shadow-xl transition-colors hover:bg-black/90 focus:outline-none focus-visible:ring-2 focus-visible:ring-white"
            style={{ top: "max(1rem, env(safe-area-inset-top))", right: "max(1rem, env(safe-area-inset-right))" }}
            onClick={(event) => {
              event.stopPropagation()
              setLightboxIndex(null)
            }}
            aria-label="关闭图片预览"
            autoFocus
          >
            <X className="h-5 w-5" />
            <span>关闭</span>
          </button>
          <span className="pointer-events-none fixed bottom-5 left-1/2 -translate-x-1/2 rounded-full bg-black/55 px-3 py-1.5 text-xs text-white/75">
            点击空白处关闭
          </span>
        </div>,
        document.body,
      )
    : null

  return (
    <article id={`thought-${thought.slug}`} className="scroll-mt-28 border-b border-border/60 py-6 first:pt-2 last:border-b-0 sm:py-8 onload-animation" style={{ animationDelay: `calc(var(--content-delay) + ${index * 50}ms)` }}>
      <div className="flex items-start gap-3">
        <img src={profile.avatar} alt={profile.name} width={40} height={40} className="mt-0.5 h-10 w-10 shrink-0 rounded-full border border-border/70 bg-muted object-cover" />
        <h3 className="pt-0.5 text-[15px] font-semibold leading-5 text-foreground">{profile.name}</h3>
      </div>

      <div className="mx-auto mt-3 w-[94%] max-w-2xl sm:w-full">
        <div>
          {thought.title ? <h4 className="mb-1.5 text-[15px] font-medium leading-6 text-foreground">{thought.title}</h4> : null}
          <div className={cn("relative overflow-hidden text-left text-[15px] leading-[1.65] text-foreground/90 transition-[max-height] duration-300 [&_a]:text-primary [&_a]:underline-offset-2 [&_a:hover]:underline [&_blockquote]:border-l-primary [&_blockquote]:text-foreground/70 [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:text-[13px] [&_li]:my-1 [&_p]:my-1 [&_p:first-child]:mt-0 [&_p:last-child]:mb-0", hasMore && !expanded ? "max-h-[11.5rem]" : "max-h-[5000px]")} dangerouslySetInnerHTML={{ __html: thought.content }} />
          {hasMore && !expanded ? <div className="relative -mt-10 flex h-10 items-end bg-gradient-to-t from-background via-background/95 to-transparent pt-5"><button type="button" className="inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline" onClick={() => setExpanded(true)}>全文 <ChevronDown className="h-4 w-4" /></button></div> : null}
          {hasMore && expanded ? <button type="button" className="mt-2 inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline" onClick={() => setExpanded(false)}>收起 <ChevronUp className="h-4 w-4" /></button> : null}
        </div>

        <ThoughtMedia images={thought.images} onOpen={setLightboxIndex} />
        {thought.tags.length ? <div className="mt-4 flex flex-wrap gap-x-3 gap-y-1.5">{thought.tags.map((tag) => <a key={tag} href={`/thoughts/?tag=${encodeURIComponent(tag)}#thoughts-main`} className="text-sm text-primary hover:underline">#{tag}</a>)}</div> : null}

        <div className="mt-3 flex flex-wrap items-center gap-1 border-t border-border/50 pt-2.5 text-xs text-muted-foreground">
          <time dateTime={thought.date} title={fullDateLabel(thought.date)} className="mr-auto px-2 py-1.5">{dateHeading(thought.date)} · {timeLabel(thought.date)}</time>
          <button type="button" className="inline-flex items-center gap-1.5 rounded-md px-2 py-1.5 transition-colors hover:bg-muted hover:text-foreground" onClick={() => void copyLink()}>{copied ? <Check className="h-3.5 w-3.5 text-green-500" /> : <Copy className="h-3.5 w-3.5" />}{copied ? "已复制" : "复制链接"}</button>
          <a href={href} className="inline-flex items-center gap-1.5 rounded-md px-2 py-1.5 transition-colors hover:bg-muted hover:text-foreground"><LinkIcon className="h-3.5 w-3.5" /> 查看原文</a>
          {thought.images.length ? <span className="ml-auto inline-flex items-center gap-1.5 px-2 py-1.5"><ImageIcon className="h-3.5 w-3.5" /> {thought.images.length} 张图片</span> : null}
        </div>
      </div>

      {lightbox}
    </article>
  )
}

export function ThoughtList({ thoughts, tags: sidebarTags, pagination, heading = "闪念", subtitle = "灵感拾遗" }: { thoughts: ThoughtMeta[]; tags: string[]; pagination?: PaginationMeta; heading?: string; subtitle?: string }) {
  const [activeTag, setActiveTag] = useState<string | null>(null)
  const [currentPage, setCurrentPage] = useState(pagination?.currentPage ?? 1)
  const basePath = pagination?.basePath ?? "/thoughts"

  const syncFromLocation = useCallback(() => {
    if (typeof window === "undefined") return
    const params = new URLSearchParams(window.location.search)
    const queryTag = params.get("tag")?.trim()
    const nextTag = queryTag && sidebarTags.includes(queryTag) ? queryTag : null
    setActiveTag(nextTag)
    setCurrentPage(nextTag ? 1 : pagination?.currentPage ?? 1)
  }, [sidebarTags, pagination?.currentPage])

  useEffect(() => {
    syncFromLocation()
    window.addEventListener("popstate", syncFromLocation)
    document.addEventListener("astro:page-load", syncFromLocation)
    return () => {
      window.removeEventListener("popstate", syncFromLocation)
      document.removeEventListener("astro:page-load", syncFromLocation)
    }
  }, [syncFromLocation])

  const filteredThoughts = useMemo(() => thoughts.filter((thought) => !activeTag || thought.tags.includes(activeTag)), [thoughts, activeTag])
  const pageSize = pagination?.pageSize ?? Math.max(1, thoughts.length)
  const totalPages = pagination ? Math.max(1, Math.ceil(filteredThoughts.length / pageSize)) : 1
  const pagedThoughts = pagination ? filteredThoughts.slice((currentPage - 1) * pageSize, currentPage * pageSize) : filteredThoughts
  const pageRange = useMemo(() => (pagination ? buildPageRange(currentPage, totalPages) : []), [pagination, currentPage, totalPages])

  const groups = useMemo(() => {
    const result: { key: string; label: string; thoughts: ThoughtMeta[] }[] = []
    for (const thought of pagedThoughts) {
      const key = dateKey(thought.date)
      const existing = result.find((group) => group.key === key)
      if (existing) existing.thoughts.push(thought)
      else result.push({ key, label: dateHeading(thought.date), thoughts: [thought] })
    }
    return result
  }, [pagedThoughts])

  const updateSearchParams = (nextTag: string | null) => {
    if (typeof window === "undefined") return
    const url = new URL(window.location.href)
    const targetPath = pagination ? getPageHref(1, basePath) : url.pathname
    if (nextTag) url.searchParams.set("tag", nextTag)
    else url.searchParams.delete("tag")
    const search = url.searchParams.toString()
    window.history.replaceState({}, "", `${targetPath}${search ? `?${search}` : ""}#thoughts-main`)
  }

  const handleTagChange = (tag: string | null) => {
    setActiveTag(tag)
    setCurrentPage(1)
    updateSearchParams(tag)
    document.getElementById("thoughts-main")?.scrollIntoView({ behavior: "smooth", block: "start" })
  }

  const handlePageClick = (page: number) => (event: MouseEvent<HTMLAnchorElement>) => {
    if (!pagination || !activeTag) return
    event.preventDefault()
    setCurrentPage(page)
  }

  return (
    <section id="thoughts-feed" className="thought-list-root px-5 py-8 sm:px-6 sm:py-12">
      <div className="mx-auto max-w-3xl">
        <div className="mb-8 overflow-hidden rounded-2xl border border-border/60 bg-card shadow-sm onload-animation">
          <div className="relative h-36 overflow-hidden bg-[radial-gradient(circle_at_20%_20%,hsl(var(--primary)/0.24),transparent_35%),linear-gradient(135deg,hsl(var(--primary)/0.16),hsl(var(--muted)),hsl(var(--background)))] sm:h-44"><div className="absolute -right-10 -top-20 h-56 w-56 rounded-full border border-primary/10 bg-primary/5 blur-2xl" /><div className="absolute bottom-0 left-0 h-20 w-full bg-gradient-to-t from-card/60 to-transparent" /></div>
          <div className="relative px-5 pb-5 sm:px-7 sm:pb-6">
            <img src={profile.avatar} alt={profile.name} width={76} height={76} className="-mt-10 h-[76px] w-[76px] rounded-full border-4 border-card bg-muted object-cover shadow-md" />
            <div className="mt-3 flex flex-wrap items-end justify-between gap-4"><div><h1 className="text-2xl font-semibold tracking-tight text-foreground">{profile.name}</h1><p className="mt-1 text-sm text-muted-foreground">{profile.bio}</p></div><div className="text-right text-xs text-muted-foreground"><div className="text-base font-semibold text-foreground">{thoughts.length}</div>条闪念</div></div>
          </div>
        </div>

        <div className="mb-8 flex flex-wrap items-end justify-between gap-4"><div><span className="block text-sm font-medium tracking-wide text-primary">{subtitle}</span><h2 className="mt-1 text-2xl font-semibold text-foreground sm:text-3xl">{heading}</h2></div>{activeTag ? <button type="button" onClick={() => handleTagChange(null)} className="text-sm text-muted-foreground hover:text-primary">清除 #{activeTag}</button> : null}</div>

        {sidebarTags.length ? <div className="mb-3 flex gap-2 overflow-x-auto pb-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"><button type="button" onClick={() => handleTagChange(null)} className={cn("shrink-0 rounded-full border px-3 py-1.5 text-xs transition-colors", !activeTag ? "border-primary bg-primary text-primary-foreground" : "border-border bg-card text-muted-foreground hover:border-primary/50 hover:text-foreground")}>全部</button>{sidebarTags.map((tag) => <button key={tag} type="button" onClick={() => handleTagChange(activeTag === tag ? null : tag)} className={cn("shrink-0 rounded-full border px-3 py-1.5 text-xs transition-colors", activeTag === tag ? "border-primary bg-primary text-primary-foreground" : "border-border bg-card text-muted-foreground hover:border-primary/50 hover:text-foreground")}>#{tag}</button>)}</div> : null}

        <div className="border-t border-border/60">{groups.length ? groups.map((group) => <section key={group.key} aria-label={group.label}><div className="flex items-center gap-3 py-4 text-xs font-medium text-muted-foreground"><span className="h-px flex-1 bg-border/60" /><span>{group.label}</span><span className="h-px flex-1 bg-border/60" /></div>{group.thoughts.map((thought, index) => <ThoughtCard key={thought.slug} thought={thought} index={index} />)}</section>) : <div className="py-16 text-center text-sm text-muted-foreground">暂无闪念</div>}</div>

        {pagination && totalPages > 1 ? <div className="pt-8"><Pagination><PaginationContent><PaginationItem><PaginationLink href={currentPage > 1 ? `${getPageHref(currentPage - 1, basePath)}#thoughts-main` : undefined} aria-disabled={currentPage <= 1} tabIndex={currentPage > 1 ? undefined : -1} className={cn("gap-1 px-2.5", currentPage <= 1 && "pointer-events-none opacity-50")} onClick={currentPage > 1 ? handlePageClick(currentPage - 1) : undefined}><ChevronLeft className="size-4" /><span className="hidden sm:block">上一页</span></PaginationLink></PaginationItem>{pageRange.map((page, index) => <PaginationItem key={`${page}-${index}`}>{page === HIDDEN ? <PaginationEllipsis /> : <PaginationLink href={`${getPageHref(page, basePath)}#thoughts-main`} isActive={currentPage === page} onClick={handlePageClick(page)}>{page}</PaginationLink>}</PaginationItem>)}<PaginationItem><PaginationLink href={currentPage < totalPages ? `${getPageHref(currentPage + 1, basePath)}#thoughts-main` : undefined} aria-disabled={currentPage >= totalPages} tabIndex={currentPage < totalPages ? undefined : -1} className={cn("gap-1 px-2.5", currentPage >= totalPages && "pointer-events-none opacity-50")} onClick={currentPage < totalPages ? handlePageClick(currentPage + 1) : undefined}><span className="hidden sm:block">下一页</span><ChevronRight className="size-4" /></PaginationLink></PaginationItem></PaginationContent></Pagination></div> : null}
      </div>
    </section>
  )
}
