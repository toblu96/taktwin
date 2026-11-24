import { resolve } from 'node:path'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import icons from 'unplugin-icons/vite'
import wails from '@wailsio/runtime/plugins/vite'

const codemirror = [
  '@codemirror/autocomplete',
  '@codemirror/commands',
  '@codemirror/language',
  '@codemirror/lint',
  '@codemirror/search',
  '@codemirror/state',
  '@codemirror/view',
  '@lezer/common',
  '@lezer/highlight',
]

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: '127.0.0.1',
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  // Icons are compiled into the bundle, so the app never fetches one.
  plugins: [vue(), tailwindcss(), icons({ compiler: 'vue3' }), wails('./bindings')],
  // The code editor's packages share state through @lezer/common and @lezer/highlight, so each
  // must exist once. The dev server bundles them together when it starts, rather than when the
  // programming tool first loads, which would make second copies that cannot highlight.
  resolve: { dedupe: codemirror },
  optimizeDeps: { include: codemirror },
  build: {
    rollupOptions: {
      // lab.html is the UI spike: the new look tried out on its own, apart from the app.
      input: { main: resolve(__dirname, 'index.html'), lab: resolve(__dirname, 'lab.html') },
    },
  },
})
