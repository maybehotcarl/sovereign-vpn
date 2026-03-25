import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { loadEnv } from 'vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, __dirname, '')
  const gatewayProxyTarget = env.VITE_GATEWAY_PROXY_TARGET || 'http://localhost:8080'
  const gatewayPublicTarget =
    env.VITE_GATEWAY_PUBLIC_TARGET || gatewayProxyTarget

  return {
    define: {
      global: 'globalThis',
    },
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
        '/health': gatewayProxyTarget,
        '/auth': gatewayProxyTarget,
        '/vpn': gatewayProxyTarget,
        '/nodes': gatewayPublicTarget,
        '/session': gatewayPublicTarget,
        '/subscription': gatewayPublicTarget,
        '/payout': gatewayProxyTarget,
      },
    },
  }
})
