import { defineConfig } from 'astro/config'
import react from '@astrojs/react'

export default defineConfig({
  srcDir: './src',
  output: 'static',
  alias: {
    '@': './src',
  },
  integrations: [react()],
})
