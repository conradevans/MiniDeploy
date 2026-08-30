import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],

  server: {
    host: '127.0.0.1',

    proxy: {
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
