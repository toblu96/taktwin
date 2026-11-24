import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import IdeWindow from './ide/IdeWindow.vue'
import './ui/theme.css'
import { keyboardReach } from './ui/keyboard'

// Dark until light mode is checked on every page.
document.documentElement.dataset.theme = 'dark'

// A window opened for a PLC's programming tool shows only that.
const plc = new URLSearchParams(location.search).get('ide')
if (plc !== null) createApp(IdeWindow, { plc }).mount('#app')
else createApp(App).use(router).mount('#app')
keyboardReach()
