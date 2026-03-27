import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:3100',
      '/vm': {
        target: 'http://127.0.0.1:8428',
        rewrite: (path) => path.replace(/^\/vm/, ''),
      },
      '/ws': { target: 'ws://127.0.0.1:3100', ws: true },
    },
  },
})
