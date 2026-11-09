import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// In development the admin API is proxied, so the console and the API share an
// origin exactly as they do behind the console image's nginx.
const admin = process.env.HAYAT_ADMIN_URL ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(import.meta.dirname, 'src') } },
  server: {
    port: 3000,
    proxy: {
      '/hayat.admin.v1.': { target: admin, changeOrigin: true },
      '/config.json': { target: admin, changeOrigin: true },
    },
  },
})
