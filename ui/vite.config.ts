/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The Go server (`agnostic-ai serve`) listens on 4310 and answers /api.
// Dev and preview both forward /api there, so the client always talks to
// the same paths it uses when the server hosts the built files itself.
const api = { '/api': { target: process.env.AGNOSTIC_AI_API_URL ?? 'http://127.0.0.1:4310', changeOrigin: false } }

export default defineConfig({
  plugins: [react()],
  // Relative asset paths let the built files run from any directory,
  // including a static snapshot hosted on GitHub Pages.
  base: './',
  server: { proxy: api },
  preview: { proxy: api },
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node',
  },
})
