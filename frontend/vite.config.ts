import { resolve } from 'node:path'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import icons from 'unplugin-icons/vite'
import wails from '@wailsio/runtime/plugins/vite'

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: '127.0.0.1',
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  // Icons are compiled into the bundle, so the app never fetches one.
  plugins: [vue(), tailwindcss(), icons({ compiler: 'vue3' }), wails('./bindings')],
  build: {
    rollupOptions: {
      // lab.html is the UI spike: the new look tried out on its own, apart from the app.
      input: { main: resolve(__dirname, 'index.html'), lab: resolve(__dirname, 'lab.html') },
    },
  },
})
