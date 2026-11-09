/// <reference types="vitest/config" />
import { writeFileSync } from 'node:fs'
import { fileURLToPath, URL } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, type Plugin } from 'vite'

// The Go binary embeds web/dist; vite empties it on build, so restore the
// placeholder that keeps `go build` working on a fresh checkout.
const keepGitkeep = (): Plugin => ({
  name: 'acksy-keep-gitkeep',
  apply: 'build',
  closeBundle() {
    writeFileSync(fileURLToPath(new URL('./dist/.gitkeep', import.meta.url)), '')
  },
})

const backend = process.env.ACKSY_DEV_BACKEND ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [tanstackRouter({ target: 'react', autoCodeSplitting: true }), react(), tailwindcss(), keepGitkeep()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: backend, changeOrigin: true, ws: true },
      '/healthz': backend,
      '/readyz': backend,
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['src/test/setup.ts'],
  },
})
