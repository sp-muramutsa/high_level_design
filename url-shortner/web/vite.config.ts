import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      // Proxy short code redirects if not matching assets
      '^/[a-zA-Z0-9_-]{1,32}$': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      }
    }
  }
})
