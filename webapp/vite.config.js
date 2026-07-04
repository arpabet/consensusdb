import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Two apps, one build, shared assets under /assets:
//   index.html   → the read-only dashboard, served by the node at /
//   console.html → the admin console,       served by the node at /console
// Both call the admin REST API under /api. In dev, proxy /api to a running node.
export default defineConfig({
  plugins: [vue()],
  base: '/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      input: {
        index: 'index.html',
        console: 'console.html',
      },
    },
  },
  server: {
    proxy: {
      '/api': { target: 'http://localhost:8441', changeOrigin: true },
    },
  },
})
