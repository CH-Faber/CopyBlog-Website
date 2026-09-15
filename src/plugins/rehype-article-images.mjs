const text = (value) => ({ type: "text", value })

const classNames = (properties = {}) => {
  const current = properties.className
  if (Array.isArray(current)) return current.map(String)
  if (typeof current === "string" && current.trim()) return current.trim().split(/\s+/)
  return []
}

const isArticleImage = (child) => {
  if (child?.type !== "element" || child.tagName !== "img") return false
  return String(child.properties?.src ?? "").startsWith("/obsidian-assets/")
}

const imageChildren = (paragraph) => {
  if (!paragraph || paragraph.type !== "element" || paragraph.tagName !== "p") return null
  const children = paragraph.children ?? []
  const images = []
  let cursor = 0

  while (cursor < children.length) {
    const child = children[cursor]
    if (isArticleImage(child)) {
      images.push(child)
      cursor += 1
      continue
    }

    if (child.type === "text" && !child.value.trim()) {
      const hasLineBreak = /\r?\n/.test(child.value)
      const next = children[cursor + 1]
      if (isArticleImage(next)) {
        cursor += 1
        continue
      }
      if (images.length && hasLineBreak) {
        return { images, remainder: children.slice(cursor + 1) }
      }
      return images.length && cursor === children.length - 1 ? { images, remainder: [] } : null
    }

    if (images.length && child.type === "text" && /^[\t ]*\r?\n/.test(child.value)) {
      const remainder = children.slice(cursor)
      remainder[0] = { ...child, value: child.value.replace(/^[\t ]*\r?\n[\t ]*/, "") }
      return { images, remainder: remainder.filter((node, index) => index > 0 || node.value) }
    }

    return null
  }

  return images.length ? { images, remainder: [] } : null
}

const imageButton = (image, index, total) => {
  image.properties ??= {}
  image.properties.className = [...classNames(image.properties), "article-media-image"]
  image.properties.loading = "lazy"
  image.properties.decoding = "async"
  image.properties.draggable = "false"

  const source = String(image.properties.src ?? "")
  const caption = String(image.properties.title ?? "").trim()
  delete image.properties.title

  return {
    type: "element",
    tagName: "button",
    properties: {
      type: "button",
      className: ["article-image-trigger"],
      "data-article-image": "true",
      "data-image-src": source,
      "data-image-caption": caption,
      ariaLabel: caption ? `查看大图：${caption}` : `查看第 ${index + 1} 张图片，共 ${total} 张`,
    },
    children: [image],
  }
}

const figureFor = (images) => {
  if (images.length === 1) {
    const image = images[0]
    const caption = String(image.properties?.title ?? "").trim()
    const children = [imageButton(image, 0, 1)]
    if (caption) {
      children.push({
        type: "element",
        tagName: "figcaption",
        properties: { className: ["article-image-caption"] },
        children: [text(caption)],
      })
    }
    return {
      type: "element",
      tagName: "figure",
      properties: { className: ["article-media", "article-figure"] },
      children,
    }
  }

  const layout = images.length <= 4 ? String(images.length) : "many"
  return {
    type: "element",
    tagName: "figure",
    properties: {
      className: ["article-media", "article-gallery", `article-gallery--${layout}`],
      "data-image-count": String(images.length),
      ariaLabel: `包含 ${images.length} 张图片的照片墙`,
    },
    children: images.map((image, index) => imageButton(image, index, images.length)),
  }
}

const transformChildren = (parent) => {
  if (!parent || !Array.isArray(parent.children)) return
  for (let index = 0; index < parent.children.length; index += 1) {
    const child = parent.children[index]
    const media = imageChildren(child)
    if (media) {
      const replacements = [figureFor(media.images)]
      if (media.remainder.length) replacements.push({ ...child, children: media.remainder })
      parent.children.splice(index, 1, ...replacements)
      index += replacements.length - 1
      continue
    }
    transformChildren(child)
  }
}

export function rehypeArticleImages() {
  return (tree) => transformChildren(tree)
}
