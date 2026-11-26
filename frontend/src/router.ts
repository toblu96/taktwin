import { ref } from 'vue'
import { createRouter, createWebHashHistory, type RouteLocationRaw } from 'vue-router'
import { edits } from './builder'
import { expert, expertRead } from './expert'
import { project, ready, reloadStation } from './sim'

// Where the app is: its projects, an open project, the library, or the settings. The address
// lives after the # of the window's page, so a reload of the window stays where it was and
// needs nothing from the server that serves the page.
//
// An open project has three views: the simulation, the wiring and, on an exercise, the exercise,
// which a project on none does not have. Each runs, as a
// student uses it, or, in expert mode, is edited: the simulation's edit is the builder, the
// wiring's is rewiring. Writing an exercise comes later, so the exercise only runs.
export type SettingsTab = 'general' | 'collections' | 'updates' | 'about'
export type ProjectView = 'simulation' | 'wiring' | 'exercise'

declare module 'vue-router' {
  interface RouteMeta {
    project?: boolean
    view?: ProjectView
    edit?: boolean
  }
}

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'projects', component: () => import('./components/ProjectsPage.vue') },
    { path: '/project', redirect: { name: 'simulate' } },
    { path: '/project/build', redirect: { name: 'build' } },
    { path: '/project/simulation', name: 'simulate', component: () => import('./components/ProjectWindow.vue'), meta: { project: true, view: 'simulation' } },
    { path: '/project/simulation/edit', name: 'build', component: () => import('./components/BuilderPage.vue'), meta: { project: true, view: 'simulation', edit: true } },
    { path: '/project/wiring', name: 'wiring', component: () => import('./components/WiringPage.vue'), meta: { project: true, view: 'wiring' } },
    { path: '/project/wiring/edit', name: 'rewire', component: () => import('./components/WiringPage.vue'), meta: { project: true, view: 'wiring', edit: true } },
    { path: '/project/exercise', name: 'exercise', component: () => import('./components/ExercisePage.vue'), meta: { project: true, view: 'exercise' } },
    { path: '/library/:level(station|module|accessory|part)?/:id?', name: 'library', component: () => import('./components/LibraryPage.vue') },
    { path: '/settings/:tab(general|collections|updates|about)?', name: 'settings', component: () => import('./components/SettingsPage.vue') },
    { path: '/:rest(.*)*', redirect: '/' },
  ],
})

// What went wrong on the way to a view of the open project, for the top bar to show.
export const viewProblem = ref('')

// Whether the views are edited, which the switch in the top bar sets and the address shows. It
// stays while going between views, so the simulation and the wiring are both edited until it is
// switched off; the exercise only runs so far.
export const editing = ref(false)
router.afterEach((to) => {
  if (to.meta.project && to.meta.view !== 'exercise') editing.value = !!to.meta.edit
  if (!to.meta.project && to.name !== 'settings') editing.value = false
})

// The address of a view of the open project, run or edited.
export function projectRoute(view: ProjectView, edit: boolean) {
  if (view === 'exercise') return { name: 'exercise' }
  if (view === 'wiring') return { name: edit ? 'rewire' : 'wiring' }
  return { name: edit ? 'build' : 'simulate' }
}

// The pages of a project need one open, and the projects page shows only while none is. Only
// expert mode edits, never an exercise's station. After editing, the station loads again as it
// was built.
let built = false
router.beforeEach(async (to, from) => {
  await Promise.all([ready, expertRead])
  const p = project.value
  if (to.meta.project && !p) return { name: 'projects' }
  if (to.name === 'projects' && p) return { name: 'simulate' }
  if (to.meta.view === 'exercise' && !p?.exercise) return { name: 'simulate' }
  if (to.meta.edit && (!expert.value || !p || p.exercise)) return projectRoute(to.meta.view ?? 'simulation', false)
  // Changes made while editing, also in one view and then another, load the station again
  // once it runs.
  if (from.meta.edit && edits.value) built = true
  if (to.meta.project && !to.meta.edit && built) {
    built = false
    viewProblem.value = ''
    try {
      await reloadStation()
    } catch (e) {
      viewProblem.value = `The station does not load: ${(e as Error).message ?? e}`
    }
  }
})

export const go = (to: RouteLocationRaw) => router.push(to)

// The settings' tab that showed last, where they open again while the app runs.
export const lastSettingsTab = ref<SettingsTab>('general')
router.afterEach((to) => {
  if (to.name === 'settings' && to.params.tab) lastSettingsTab.value = to.params.tab as SettingsTab
})

// The settings open on a tab, or on the one they showed last; going between their tabs does
// not add to the way back, so that closing them returns to where the app was before.
export function openSettings(tab?: SettingsTab) {
  const to = { name: 'settings', params: { tab: tab ?? lastSettingsTab.value } }
  return router.currentRoute.value.name === 'settings' ? router.replace(to) : router.push(to)
}

export function closeSettings() {
  if (router.currentRoute.value.name !== 'settings') return
  const back = router.options.history.state.back
  if (typeof back === 'string' && !back.startsWith('/settings')) router.back()
  else void router.push(project.value ? { name: 'simulate' } : { name: 'projects' })
}

export const toggleSettings = () => (router.currentRoute.value.name === 'settings' ? closeSettings() : openSettings())
