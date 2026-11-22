// The UI lab: the app's components from src/ui on their own, and layouts in the new look tried
// out before they reach the app, in dark and light.
import '@ui-sdk/core/web.css'
import '../ui/theme.css'
import { createApp } from 'vue'
import Lab from './Lab.vue'

createApp(Lab).mount('#lab')
