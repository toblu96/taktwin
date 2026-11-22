import { ref } from 'vue'
import { createRouter, createWebHashHistory, type RouteLocationRaw } from 'vue-router'
import { edits } from './builder'
import { viewProblem } from './project-view'
import { project, ready, reloadStation } from './sim'

// Where the app is: its projects, an open project being simulated or built, the library, or
// the settings. The address lives after the # of the window's page, so a reload of the window
// stays where it was and needs nothing from the server that serves the page.
export type SettingsTab = 'general' | 'collections' | 'updates' | 'about'

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'projects', component: () => import('./components/ProjectsPage.vue') },
    { path: '/project', name: 'simulate', component: () => import('./components/ProjectWindow.vue'), meta: { project: true } },
    { path: '/project/build', name: 'build', component: () => import('./components/BuilderPage.vue'), meta: { project: true } },
    { path: '/library', name: 'library', component: () => import('./components/LibraryPage.vue') },
    { path: '/settings/:tab(general|collections|updates|about)?', name: 'settings', component: () => import('./components/SettingsPage.vue') },
    { path: '/:rest(.*)*', redirect: '/' },
  ],
})

// The pages of a project need one open, and the projects page shows only while none is. An
// exercise's station is not built on, and a station from a collection only once it is the
// project's own copy, which the activity bar asks for.
let built = false
router.beforeEach(async (to, from) => {
  await ready
  const p = project.value
  if (to.meta.project && !p) return { name: 'projects' }
  if (to.name === 'projects' && p) return { name: 'simulate' }
  if (to.name === 'build' && (p?.exercise || !p?.own)) return { name: 'simulate' }
  // Back from building, the simulation runs the station as it was built.
  if (from.name === 'build' && edits.value) built = true
  if (to.name === 'simulate' && built) {
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
