import { defineCollection, z } from 'astro:content'

const articles = defineCollection({
  type: 'content',
  schema: z.object({
    title: z.string(),
    excerpt: z.string().optional(),
    category: z.string(),
    categoryLabel: z.string(),
    tag: z.string(),
    date: z.coerce.date(),
    wordCount: z.number().optional(),
    readTime: z.string().optional(),
    image: z.string().optional(),
    pinned: z.boolean().optional(),
  }),
})

export const collections = { articles }
