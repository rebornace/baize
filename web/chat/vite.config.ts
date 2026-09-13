/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  base: '/ui/',
  plugins: [react()],
  server: { proxy: { '/v0': 'http://127.0.0.1:8080' } },
  build: { outDir: '../../internal/ui/dist', emptyOutDir: true },
  test: { environment: 'node' },
})
