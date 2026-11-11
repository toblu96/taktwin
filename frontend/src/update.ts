import { Events } from '@wailsio/runtime'
import { computed, ref } from 'vue'
import { UpdateService } from '../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { UpdateStatus } from '../bindings/github.com/toblu96/taktwin/internal/simapp/models'

// The app's updates: what Settings shows and the Settings tab's dot, kept current by the
// events of Wails' updater.
export const update = ref<UpdateStatus | null>(null)
export const progress = ref<{ written: number; total: number } | null>(null)
export const updating = ref(false)
export const updateFailed = ref('')

// An update is there: found by a check, or downloaded and waiting for a restart.
export const updateAvailable = computed(() => !!update.value?.latest)

export async function refreshUpdate() {
  update.value = await UpdateService.Status()
}

async function run(what: () => Promise<UpdateStatus>) {
  updating.value = true
  updateFailed.value = ''
  try {
    update.value = await what()
  } catch (e) {
    updateFailed.value = e instanceof Error ? e.message : String(e)
    await refreshUpdate()
  } finally {
    updating.value = false
  }
}

export const checkForUpdate = () => run(() => UpdateService.Check())
export const installUpdate = () => {
  progress.value = { written: 0, total: 0 }
  return run(() => UpdateService.Install()).finally(() => (progress.value = null))
}
export const restartForUpdate = () => UpdateService.Restart()
export const openRelease = (url: string) => UpdateService.OpenRelease(url)
export async function setAutoCheck(on: boolean) {
  await UpdateService.SetAutoCheck(on)
  await refreshUpdate()
}

export function connectUpdates() {
  void refreshUpdate()
  // A check at the start finds an update without Settings being open.
  Events.On('wails:updater:update-available', () => void refreshUpdate())
  Events.On('wails:updater:download-progress', (event) => {
    const p = event.data as { written: number; total: number }
    progress.value = { written: p.written, total: p.total }
  })
}
