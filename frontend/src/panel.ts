import { computed, ref } from 'vue'
import { stored } from './ui/stored'

// The panel below a page's middle, as VS Code has one, which pages share: whether it is open and
// how high it is are the same on every page and kept between starts. A page that has something
// for it shows a panel (PagePanel) with its own tabs; ⌘J or Ctrl+J and Panel in the status bar
// show and hide it there, and do nothing on a page without one.
export const panelOpen = stored('panel.open', true)

const shown = ref(0)
export const panelHere = computed(() => shown.value > 0)
export const panelArrived = () => shown.value++
export const panelLeft = () => (shown.value = Math.max(0, shown.value - 1))

export function togglePanel() {
  if (panelHere.value) panelOpen.value = !panelOpen.value
}

// ⌘J or Ctrl+J, wherever the focus is.
export function panelKeys() {
  window.addEventListener('keydown', (e) => {
    if (!(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== 'j' || !panelHere.value) return
    e.preventDefault()
    togglePanel()
  })
}
