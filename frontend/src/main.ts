import { createApp } from 'vue'
import App from './App.vue'
import IdeWindow from './ide/IdeWindow.vue'
import './styles.css'

// A window opened for a PLC's programming tool shows only that.
const plc = new URLSearchParams(location.search).get('ide')
if (plc !== null) createApp(IdeWindow, { plc }).mount('#app')
else createApp(App).mount('#app')
