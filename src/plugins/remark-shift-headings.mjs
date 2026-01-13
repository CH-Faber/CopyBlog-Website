import { visit } from "unist-util-visit"

export function remarkShiftHeadings(options = {}) {
  const {
    shift = 1,
    onlyIn = ["/src/content/posts/"]
  } = options

  return (tree, file) => {
    const rawPath = typeof file?.path === "string" ? file.path : ""
    const normalizedPath = rawPath.replace(/\\/g, "/")
    const shouldShift = onlyIn.some((segment) => normalizedPath.includes(segment))

    if (!shouldShift || shift <= 0) return

    visit(tree, "heading", (node) => {
      if (typeof node.depth !== "number") return
      node.depth = Math.min(node.depth + shift, 6)
    })
  }
}
