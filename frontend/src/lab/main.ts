// The UI lab: Taktwin's new look tried out on its own page, apart from the app, with the same
// components on UI SDK and on Reka UI to compare.
import '@ui-sdk/core/web.css'
import './theme.css'
import { createApp } from 'vue'
import Lab from './Lab.vue'

createApp(Lab).mount('#lab')
