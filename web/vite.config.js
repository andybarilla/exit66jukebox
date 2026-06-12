import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  build: { outDir: '../internal/web/dist', emptyOutDir: true },
  server: {
    proxy: {
      '/api': 'http://localhost:8066',
      '/stream': 'http://localhost:8066',
    },
  },
})
