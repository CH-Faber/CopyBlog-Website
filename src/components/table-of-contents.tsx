"use client"

import { useState, useEffect, useRef, useCallback } from "react"
import { cn } from "@/lib/utils"

interface TocItem {
  id: string
  text: string
  level: number
}

interface TableOfContentsProps {
  showHeader?: boolean
}

export function TableOfContents({ showHeader = true }: TableOfContentsProps) {
  const [headings, setHeadings] = useState<TocItem[]>([])
  const [activeId, setActiveId] = useState<string>("")
  const navRef = useRef<HTMLElement | null>(null)
  const observerRef = useRef<IntersectionObserver | null>(null)
  const headingElementsRef = useRef<Map<string, IntersectionObserverEntry>>(new Map())
  const programmaticTargetRef = useRef<string>("")
  const programmaticLockRef = useRef(false)
  const lockTimerRef = useRef<number | null>(null)

  const unlockProgrammaticLock = useCallback(() => {
    programmaticLockRef.current = false
    programmaticTargetRef.current = ""
    if (lockTimerRef.current !== null) {
      window.clearTimeout(lockTimerRef.current)
      lockTimerRef.current = null
    }
  }, [])

  const collectHeadings = useCallback(() => {
    const article = document.querySelector("article")
    if (!article) return

    const elements = article.querySelectorAll("h2, h3")
    const items: TocItem[] = []
    elements.forEach((el) => {
      const id = el.id
      if (!id) return
      const clone = el.cloneNode(true) as HTMLElement
      clone.querySelectorAll(".anchor, .anchor-icon").forEach((a) => a.remove())
      const text = clone.textContent?.trim() || ""
      if (!text) return
      const level = parseInt(el.tagName[1], 10)
      items.push({ id, text, level })
    })
    setHeadings(items)
    return items
  }, [])

  const setupObserver = useCallback(
    (items: TocItem[]) => {
      if (observerRef.current) {
        observerRef.current.disconnect()
      }
      headingElementsRef.current = new Map()

      observerRef.current = new IntersectionObserver(
        (entries) => {
          entries.forEach((entry) => {
            headingElementsRef.current.set(entry.target.id, entry)
          })

          if (programmaticLockRef.current) {
            const targetId = programmaticTargetRef.current
            if (!targetId) {
              unlockProgrammaticLock()
            } else {
              const targetEl = document.getElementById(targetId)
              if (!targetEl) {
                unlockProgrammaticLock()
              } else {
                const targetTop = targetEl.getBoundingClientRect().top
                const expectedTop = 80
                const unlockTolerance = 18

                if (Math.abs(targetTop - expectedTop) > unlockTolerance) {
                  setActiveId(targetId)
                  return
                }

                unlockProgrammaticLock()
              }
            }
          }

          const visibleHeadings: IntersectionObserverEntry[] = []
          headingElementsRef.current.forEach((entry) => {
            if (entry.isIntersecting) visibleHeadings.push(entry)
          })

          if (visibleHeadings.length > 0) {
            const sorted = visibleHeadings.sort(
              (a, b) => a.boundingClientRect.top - b.boundingClientRect.top,
            )
            setActiveId(sorted[0].target.id)
          } else {
            const scrollY = window.scrollY
            let closestId = ""
            let closestTop = -Infinity
            items.forEach((item) => {
              const el = document.getElementById(item.id)
              if (!el) return
              const top = el.getBoundingClientRect().top + scrollY
              if (top <= scrollY + 100 && top > closestTop) {
                closestId = item.id
                closestTop = top
              }
            })
            if (closestId) setActiveId(closestId)
          }
        },
        {
          rootMargin: "-80px 0px -60% 0px",
          threshold: 0,
        },
      )

      items.forEach((item) => {
        const el = document.getElementById(item.id)
        if (el) observerRef.current?.observe(el)
      })
    },
    [unlockProgrammaticLock],
  )

  useEffect(() => {
    const init = () => {
      const items = collectHeadings()
      if (items && items.length > 0) {
        setupObserver(items)
      }
    }

    init()
    document.addEventListener("astro:page-load", init)

    const handleUserIntent = () => {
      if (programmaticLockRef.current) {
        unlockProgrammaticLock()
      }
    }

    window.addEventListener("wheel", handleUserIntent, { passive: true })
    window.addEventListener("touchstart", handleUserIntent, { passive: true })
    window.addEventListener("keydown", handleUserIntent)

    return () => {
      observerRef.current?.disconnect()
      document.removeEventListener("astro:page-load", init)
      window.removeEventListener("wheel", handleUserIntent)
      window.removeEventListener("touchstart", handleUserIntent)
      window.removeEventListener("keydown", handleUserIntent)
      unlockProgrammaticLock()
    }
  }, [collectHeadings, setupObserver, unlockProgrammaticLock])

  useEffect(() => {
    if (!activeId || !navRef.current) return
    const activeLink = navRef.current.querySelector<HTMLAnchorElement>(`a[href="#${CSS.escape(activeId)}"]`)
    if (!activeLink) return

    const scrollContainer = navRef.current.closest<HTMLElement>(".toc-scroll-container")
    if (!scrollContainer) return

    const containerRect = scrollContainer.getBoundingClientRect()
    const linkRect = activeLink.getBoundingClientRect()
    const padding = 12
    const isOutOfView =
      linkRect.top < containerRect.top + padding ||
      linkRect.bottom > containerRect.bottom - padding

    if (isOutOfView) {
      activeLink.scrollIntoView({ block: "nearest", inline: "nearest" })
    }
  }, [activeId])

  const handleClick = (e: React.MouseEvent, id: string) => {
    e.preventDefault()
    const el = document.getElementById(id)
    if (!el) return

    unlockProgrammaticLock()
    programmaticLockRef.current = true
    programmaticTargetRef.current = id

    const top = el.getBoundingClientRect().top + window.scrollY - 80
    window.scrollTo({ top, behavior: "smooth" })
    setActiveId(id)

    lockTimerRef.current = window.setTimeout(() => {
      unlockProgrammaticLock()
    }, 2500)
  }

  if (headings.length === 0) return null

  return (
    <nav ref={navRef} aria-label="目录" className="toc-nav">
      {showHeader ? (
        <div className="flex items-center gap-2 mb-3">
          <div className="w-1 h-4 bg-primary rounded-full" />
          <span className="text-xs font-medium text-foreground tracking-wide">目录</span>
        </div>
      ) : null}
      <ul className="space-y-0.5">
        {headings.map((heading) => (
          <li key={heading.id}>
            <a
              href={`#${heading.id}`}
              onClick={(e) => handleClick(e, heading.id)}
              className={cn(
                "block py-1 text-[13px] leading-relaxed border-l-2 transition-all duration-200",
                heading.level === 2 ? "pl-3" : "pl-6",
                activeId === heading.id
                  ? "border-l-primary text-primary font-medium"
                  : "border-l-transparent text-muted-foreground hover:text-foreground hover:border-l-border",
              )}
            >
              <span className="line-clamp-2">{heading.text}</span>
            </a>
          </li>
        ))}
      </ul>
    </nav>
  )
}
