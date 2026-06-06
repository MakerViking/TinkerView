import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.dirname(fileURLToPath(import.meta.url))

// In dev, proxy API/WS/media to the Go backend (which itself proxies go2rtc +
// Frigate). Run the backend with `go run ./cmd/dashboard -config config.yaml`.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(root, 'src') } },
  build: { outDir: 'dist', emptyOutDir: true },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8088',
      '/ws': { target: 'ws://127.0.0.1:8088', ws: true },
      '/go2rtc': { target: 'http://127.0.0.1:8088', ws: true },
      '/frigate': 'http://127.0.0.1:8088',
    },
  },
})
