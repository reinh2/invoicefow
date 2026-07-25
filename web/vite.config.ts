import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: process.env.VITE_API_PROXY_TARGET ?? 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    css: true,
    globals: true,
    // e2e/ belongs to Playwright, which needs a running instance. Vitest would
    // otherwise collect those specs and fail on Playwright's own test().
    exclude: ['node_modules/**', 'dist/**', 'e2e/**'],
  },
});
