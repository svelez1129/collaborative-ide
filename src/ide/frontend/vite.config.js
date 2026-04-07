import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/ws': { target: 'ws://localhost:8080', ws: true },
      '/run': 'http://localhost:8080',
      '/create': 'http://localhost:8080',
      '/join': 'http://localhost:8080',
    },
  },
  build: {
    outDir: 'dist',
  },
})
