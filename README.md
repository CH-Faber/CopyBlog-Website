# Astro Blog Template

A modern, fast, and feature-rich blog template built with Astro, React, and Tailwind CSS. Derived from the highly customized [VermilionVoid](https://github.com/Lapis0x0/VermilionVoid) project.

## Features

- 🚀 **Built with Astro**: Extremely fast static site generation.
- 🎨 **Tailwind CSS & Radix UI**: Beautiful, accessible, and customizable design system.
- 📝 **Advanced Markdown/MDX**: Supports GitHub-style admonitions, KaTeX math rendering, and Expressive Code blocks.
- 🌗 **Dark Mode**: Seamless dark mode support with CSS variables.
- 🔍 **Search**: Integration with Pagefind for fast static search.
- 📡 **RSS & Sitemap**: Automatic generation for SEO.

## Getting Started

### 1. Installation

First, clone this repository and install dependencies. We recommend using `pnpm`:

```bash
pnpm install
```

### 2. Configuration

To personalize this template, you need to update a few configuration files:

- **`astro.config.mjs`**: Update the `site` property with your domain name.
- **`src/data/profile.ts`**: Update this file with your name, bio, avatar, and social links.
- **`src/data/friends.json`**: Add your friends' blog links here.

### 3. Development

Start the development server:

```bash
pnpm dev
```

Visit `http://localhost:4321` to view your site.

### 4. Writing Content

- **Posts**: Add new `.md` or `.mdx` files to `src/content/posts/`.
- **Thoughts**: Add short updates to `src/content/thoughts/`.

Check the frontmatter of existing sample posts to see available metadata fields like `title`, `description`, `tags`, etc.

### 5. Deployment

Build the static site:

```bash
pnpm build
```

The output will be in the `dist` (or `static` depending on Astro config) directory, ready to be deployed to any static hosting provider like Vercel, Cloudflare Pages, Netlify, or GitHub Pages.

**(Note: This project also includes a `cwd-api` folder for Cloudflare Workers if you wish to implement backend API features.)**

## Credits

This template is an abstracted version of the [VermilionVoid](https://github.com/Lapis0x0/VermilionVoid) blog by Lapis0x0.
