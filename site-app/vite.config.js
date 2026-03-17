import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      assert: path.resolve(__dirname, 'src/shims/assert.js'),
    },
  },
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
      '/subscription': 'http://localhost:8080',
      '/payout': 'http://localhost:8080',
    },
  },
})
