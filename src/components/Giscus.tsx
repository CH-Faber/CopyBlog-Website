"use client"

import { useEffect, useRef } from "react"
import { cn } from "@/lib/utils"

const repo = import.meta.env.PUBLIC_GISCUS_REPO ?? "Lapis0x0/blog-discussion"
const repoId = import.meta.env.PUBLIC_GISCUS_REPO_ID ?? "R_kgDONda6_g"
const category = import.meta.env.PUBLIC_GISCUS_CATEGORY ?? "General"
const categoryId = import.meta.env.PUBLIC_GISCUS_CATEGORY_ID ?? "DIC_kwDONda6_s4ClN0E"
const isEnabled = Boolean(repo && repoId && category && categoryId)

export function Giscus({ className }: { className?: string }) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!isEnabled) return
    const container = containerRef.current
    if (!container) return
    if (container.querySelector("iframe.giscus-frame")) return
    if (container.querySelector("script[data-repo]")) return

    const script = document.createElement("script")
    script.src = "https://giscus.app/client.js"
    script.async = true
    script.crossOrigin = "anonymous"
    script.setAttribute("data-repo", repo)
    script.setAttribute("data-repo-id", repoId)
    script.setAttribute("data-category", category)
    script.setAttribute("data-category-id", categoryId)
    script.setAttribute("data-mapping", "pathname")
    script.setAttribute("data-strict", "0")
    script.setAttribute("data-reactions-enabled", "0")
    script.setAttribute("data-emit-metadata", "0")
    script.setAttribute("data-input-position", "bottom")
    script.setAttribute(
      "data-theme",
      document.documentElement.classList.contains("dark") ? "dark" : "light"
    )
    script.setAttribute("data-lang", "zh-CN")
    container.appendChild(script)
  }, [])

  useEffect(() => {
    if (!isEnabled) return

    const getTheme = () =>
      document.documentElement.classList.contains("dark") ? "dark" : "light"
    const updateTheme = () => {
      const iframe = document.querySelector("iframe.giscus-frame") as HTMLIFrameElement | null
      if (!iframe) return
      iframe.contentWindow?.postMessage(
        { giscus: { setConfig: { theme: getTheme() } } },
        "https://giscus.app"
      )
    }

    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        if (mutation.attributeName === "class") {
          updateTheme()
        }
      }
    })
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] })
    window.addEventListener("load", updateTheme)
    document.addEventListener("astro:page-load", updateTheme)
    document.addEventListener("astro:after-swap", updateTheme)
    updateTheme()

    return () => {
      observer.disconnect()
      window.removeEventListener("load", updateTheme)
      document.removeEventListener("astro:page-load", updateTheme)
      document.removeEventListener("astro:after-swap", updateTheme)
    }
  }, [])

  if (!isEnabled) return null

  return <div ref={containerRef} className={cn("giscus", className)} />
}
