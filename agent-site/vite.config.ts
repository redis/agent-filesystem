import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const apiTarget = process.env.AFS_API_BASE_URL || 'http://127.0.0.1:8091'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5174,
    strictPort: false,
    proxy: {
      // forward all api calls to the running afs-control-plane.
      // override with: AFS_API_BASE_URL=http://... npm run dev
      '/v1':           { target: apiTarget, changeOrigin: true, secure: false },
      '/auth':         { target: apiTarget, changeOrigin: true, secure: false },
    },
  },
  preview: {
    port: 5174,
  },
})
