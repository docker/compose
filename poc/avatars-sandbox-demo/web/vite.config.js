import { defineConfig } from 'vite'
import reactRefresh from '@vitejs/plugin-react-refresh'

export default defineConfig({

  // ↓↓↓ change to logLevel: 'info' ↓↓↓
  logLevel: 'warn',
  // ↑↑↑ change to logLevel: 'info' ↑↑↑

  plugins: [reactRefresh()],
  server: {
    host: process.env.VITE_HOST || null,
    port: process.env.VITE_PORT || null,
    hmr: {
      clientPort: process.env.VITE_CLIENT_PORT || null
    },
    proxy: {
      '^/api': {
        target: process.env.API_URL || 'http://api',
        changeOrigin: true
      }
    }
  }
})
