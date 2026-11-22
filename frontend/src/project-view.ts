import { ref } from 'vue'
import { Service } from '../bindings/github.com/toblu96/taktwin/internal/simapp'
import { ask } from './ask'
import { stored } from './ui/stored'
import { router } from './router'
import { project, refreshProject } from './sim'

// Going between what an open project shows, Simulate or Build, for the activity bar.
// What went wrong on the way shows in the top bar.
export const viewProblem = ref('')

// Building is on a station of the project's own: a collection's station becomes a copy that the
// project keeps, after asking. An exercise's station stays as the exercise has it.
export async function showBuild() {
  const p = project.value
  if (!p || p.exercise) return
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
  await router.push({ name: 'build' })
}

// Back to the simulation, which loads the station again if it was built on (see router.ts).
export const showSimulate = () => router.push({ name: 'simulate' })

// The panel below the 3D view, as VS Code has one: open or not, and which of its tabs shows.
// Whether it is open is kept between starts; ⌘J or Ctrl+J and the status bar switch it.
export type PanelTab = 'check' | 'io' | 'problems'
export const panelOpen = stored('panel.open', true)
export const panelTab = ref<PanelTab>('io')
export function showPanel(tab: PanelTab) {
  panelTab.value = tab
  panelOpen.value = true
}
