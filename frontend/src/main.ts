import { createApp } from 'vue'
import App from './App.vue'
import IdeWindow from './ide/IdeWindow.vue'
import './ui/theme.css'
import './styles.css'
import { keyboardReach } from './ui/keyboard'

// Dark until every page has moved to the new look; the older ones still have dark colours of their own.
document.documentElement.dataset.theme = 'dark'

// A window opened for a PLC's programming tool shows only that.
const plc = new URLSearchParams(location.search).get('ide')
if (plc !== null) createApp(IdeWindow, { plc }).mount('#app')
else createApp(App).mount('#app')
keyboardReach()
