import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vitest/config'
import { fileURLToPath } from 'node:url'

// https://vite.dev/config/
export default defineConfig(({ mode }) => ({
  plugins: mode === 'test' ? [react()] : [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: { proxy: { '/api': 'http://127.0.0.1:8080' } },
  test: { environment: 'jsdom', setupFiles: ['./src/test/setup.ts'], globals: true, maxWorkers: 1, fileParallelism: false },
}))
