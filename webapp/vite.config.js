import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// The console is served under /console by the consensusdb http-server, and calls
// the admin REST API under /api. In dev, proxy /api to a running node (8441).
export default defineConfig({
  plugins: [vue()],
  base: '/console/',
  build: { outDir: 'dist', emptyOutDir: true },
  server: {
    proxy: {
      '/api': { target: 'http://localhost:8441', changeOrigin: true },
    },
  },
})
