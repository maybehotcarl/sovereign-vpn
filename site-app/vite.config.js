import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
  },
  server: {
    proxy: {
      '/health': 'http://localhost:8080',
      '/auth': 'http://localhost:8080',
      '/vpn': 'http://localhost:8080',
      '/nodes': 'http://localhost:8080',
      '/session': 'http://localhost:8080',
    },
  },
})
