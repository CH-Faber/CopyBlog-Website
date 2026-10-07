import fs from "node:fs/promises"
import path from "node:path"

const root = path.resolve("public/obsidian-assets")

async function walk(directory) {
  const entries = await fs.readdir(directory, { withFileTypes: true })
  const files = []
  for (const entry of entries) {
    const file = path.join(directory, entry.name)
    if (entry.isDirectory()) files.push(...await walk(file))
    else if (/-original\.[a-z0-9]+$/i.test(entry.name)) files.push(file)
  }
  return files
}

const originals = await walk(root).catch((error) => error.code === "ENOENT" ? [] : Promise.reject(error))
for (const original of originals) {
  const source = original.replace(/-original(?=\.[a-z0-9]+$)/i, "")
  await fs.copyFile(original, source)
}
console.log(`Restored ${originals.length} source images after build.`)
