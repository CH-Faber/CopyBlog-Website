import fs from "node:fs/promises"
import path from "node:path"
import sharp from "sharp"

const root = path.resolve("public/obsidian-assets")
const sourceExtensions = new Set([".jpg", ".jpeg", ".png", ".webp"])

async function walk(directory) {
  const entries = await fs.readdir(directory, { withFileTypes: true })
  const files = []
  for (const entry of entries) {
    const file = path.join(directory, entry.name)
    if (entry.isDirectory()) files.push(...await walk(file))
    else if (sourceExtensions.has(path.extname(entry.name).toLowerCase()) && !/-original\.|-thumb\./i.test(entry.name)) files.push(file)
  }
  return files
}

const files = await walk(root).catch((error) => {
  if (error.code === "ENOENT") return []
  throw error
})

for (const file of files) {
  const extension = path.extname(file)
  const base = file.slice(0, -extension.length)
  const original = `${base}-original${extension}`
  const encode = (image, quality) => {
    if (extension.toLowerCase() === ".png") return image.png({ compressionLevel: 9 })
    if (extension.toLowerCase() === ".webp") return image.webp({ quality })
    return image.jpeg({ quality, mozjpeg: true })
  }
  await fs.copyFile(file, original)
  await encode(sharp(original).resize({ width: 1600, height: 1600, fit: "inside", withoutEnlargement: true }), 82).toFile(`${base}.optimized${extension}`)
  await fs.rename(`${base}.optimized${extension}`, file)
  await encode(sharp(original).resize({ width: 480, height: 480, fit: "inside", withoutEnlargement: true }), 78).toFile(`${base}-thumb${extension}`)
}

console.log(`Generated responsive image variants for ${files.length} source images.`)
