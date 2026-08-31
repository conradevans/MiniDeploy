import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],

  test: {
    environment: 'jsdom',
  },

  server: {
    host: '127.0.0.1',

    proxy: {
      '/api/guest': {
        target: 'http://127.0.0.1:9003',
      },

      '/api/admin': {
        target: 'http://127.0.0.1:9003',
        headers: {
          Origin: 'https://minideploy.reactorlab.dev',
        },
      },

      '/health': {
        target: 'http://127.0.0.1:9000',
      },

      '/deploy': {
        target: 'http://127.0.0.1:9000',
        headers: {
          Origin: 'http://localhost:9000',
        },
      },

      '/deployments': {
        target: 'http://127.0.0.1:9000',
        headers: {
          Origin: 'http://localhost:9000',
        },
      },
    },
  },
})
