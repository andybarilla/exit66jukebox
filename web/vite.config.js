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
  // Scoped to test runs so the production build resolves exactly as it did
  // before; without it Svelte hands back its server build and mount() is
  // unavailable to component tests. The jsdom environment is opted into
  // per-file (see App.render.test.js), because the source-reading tests need
  // node's file: import.meta.url.
  resolve: process.env.VITEST ? { conditions: ['browser'] } : {},
})
