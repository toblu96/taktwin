import { ref } from 'vue'
import { Service } from '../bindings/github.com/toblu96/taktwin/internal/simapp'
import { ask } from './ask'
import { stored } from './ui/stored'
import { editing, projectRoute, router, viewProblem, type ProjectView } from './router'
import { project, refreshProject } from './sim'

// Going between the views of an open project, and switching between running and editing them.
export const showView = (view: ProjectView) => router.push(projectRoute(view, editing.value))

// Editing is on a station of the project's own: a collection's station becomes a copy that the
// project keeps, after asking. An exercise's station stays as the exercise has it.
export async function setEditing(on: boolean) {
  const p = project.value
  const view = router.currentRoute.value.meta.view ?? 'simulation'
  if (!p) return
  if (!on) {
    editing.value = false
    await router.push(projectRoute(view, false))
    return
  }
  if (p.exercise) return
  viewProblem.value = ''
  if (!p.own) {
    const ok = await ask(`Edit a copy of ${p.stationName} that belongs to this project? The project then no longer follows updates of the station's collection.`, 'Edit a copy')
    if (!ok) return
    try {
      await Service.CustomizeStation()
      await refreshProject()
    } catch (e) {
      viewProblem.value = (e as Error).message ?? String(e)
      return
    }
  }
  editing.value = true
  await router.push(projectRoute(view, true))
}

// The panel below the 3D view, as VS Code has one: open or not, and which of its tabs shows.
// Whether it is open is kept between starts; ⌘J or Ctrl+J and the status bar switch it.
export type PanelTab = 'io' | 'events'
export const panelOpen = stored('panel.open', true)
export const panelTab = ref<PanelTab>('io')
export function showPanel(tab: PanelTab) {
  panelTab.value = tab
  panelOpen.value = true
}
