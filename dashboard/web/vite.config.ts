import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.dirname(fileURLToPath(import.meta.url))
// Backend to proxy to; point it at a deployed instance with e.g.
// TV_BACKEND=http://tower.local:8088 npm run dev
const backend = process.env.TV_BACKEND ?? 'http://127.0.0.1:8088'

// In dev, proxy API/WS/media to the Go backend (which itself proxies go2rtc +
// Frigate). Run the backend with `go run ./cmd/dashboard -config config.yaml`.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(root, 'src') } },
  build: { outDir: 'dist', emptyOutDir: true },
  server: {
    port: 5173,
    proxy: {
      '/api': backend,
      '/ws': { target: backend.replace(/^http/, 'ws'), ws: true },
      '/go2rtc': { target: backend, ws: true },
      '/frigate': backend,
    },
  },
})
