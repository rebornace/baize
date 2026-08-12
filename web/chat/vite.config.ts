import { defineConfig } from 'vite'

export default defineConfig({
  base: '/ui/',
  server: {
    proxy: {
      '/v0': 'http://127.0.0.1:8080',
    },
  },
  build: {
    outDir: '../../internal/ui/dist',
    emptyOutDir: true,
  },
})
