import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    // Wails serves the bundle from the binary, so hashed names buy nothing
    // and make the embed list harder to read.
    assetsInlineLimit: 0,
  },
})
