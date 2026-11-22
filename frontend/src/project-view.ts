import { ref } from 'vue'
import { Service } from '../bindings/github.com/toblu96/taktwin/internal/simapp'
import { ask } from './ask'
import { edits } from './builder'
import { stored } from './ui/stored'
import { closeSettings, openSettings, page, project, refreshProject, reloadStation, view } from './sim'

// What an open project shows, Simulate or Build, and going between them, for the activity bar.
// What went wrong on the way shows in the top bar.
export const viewProblem = ref('')

// Building is on a station of the project's own: a collection's station becomes a copy that the
// project keeps, after asking. An exercise's station stays as the exercise has it.
export async function showBuild() {
  const p = project.value
  page.value = 'simulation'
  if (!p || view.value === 'build' || p.exercise) return
  viewProblem.value = ''
  if (!p.own) {
    const ok = await ask(
      `Build on a copy of ${p.stationName} that belongs to this project? The project then no longer follows updates of the station's collection.`,
      'Build on a copy',
    )
    if (!ok) return
    try {
      await Service.CustomizeStation()
      await refreshProject()
    } catch (e) {
      viewProblem.value = (e as Error).message ?? String(e)
      return
    }
  }
  view.value = 'build'
}

// Back from building, the simulation runs the station as it was built.
export async function showSimulate() {
  page.value = 'simulation'
  if (view.value === 'simulate') return
  view.value = 'simulate'
  viewProblem.value = ''
  if (!edits.value) return
  try {
    await reloadStation()
  } catch (e) {
    viewProblem.value = `The station does not load: ${(e as Error).message ?? e}`
  }
}

// The gear shows the settings, and a second click goes back to where the app was.
export const toggleSettings = () => (page.value === 'settings' ? closeSettings() : openSettings())

// The panel below the 3D view, as VS Code has one: open or not, and which of its tabs shows.
// Whether it is open is kept between starts; ⌘J or Ctrl+J and the status bar switch it.
export type PanelTab = 'check' | 'io' | 'problems'
export const panelOpen = stored('panel.open', true)
export const panelTab = ref<PanelTab>('io')
export function showPanel(tab: PanelTab) {
  panelTab.value = tab
  panelOpen.value = true
}
