import { editing, projectRoute, router, viewProblem, type ProjectView } from './router'
import { project } from './sim'

// Going between the views of an open project, and switching between running and editing them.
export const showView = (view: ProjectView) => router.push(projectRoute(view, editing.value))

// Editing is on the project's own copy of its station. An exercise's station stays as the
// exercise has it.
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
  editing.value = true
  await router.push(projectRoute(view, true))
}
