import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = fileURLToPath(new URL('.', import.meta.url))

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    extensions: ['.mjs', '.js', '.ts', '.jsx', '.tsx', '.json'],
    alias: {
      '@': resolve(__dirname, './src'),
    },
  },
  build: {
    outDir: resolve(__dirname, '../app/webapp'),
    emptyOutDir: true,
    rollupOptions: {
      onwarn(warning, warn) {
        // Suppress "use client" directive warnings
        if (warning.code === 'MODULE_LEVEL_DIRECTIVE') {
          return
        }
        warn(warning)
      },
    },
  },
  optimizeDeps: {
    include: ['react', 'react-dom'],
  },
})
