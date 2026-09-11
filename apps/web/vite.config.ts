import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// Only the local Go service owns business logic and database connections.
const proxy = { '/api': { target: process.env.ERP_API_PROXY_TARGET ?? 'http://127.0.0.1:8080', changeOrigin: false } };

export default defineConfig({
  plugins: [sveltekit()],
  server: { host: '127.0.0.1', port: 5173, strictPort: true, proxy },
  preview: { host: '127.0.0.1', port: 4173, strictPort: true, proxy }
});
