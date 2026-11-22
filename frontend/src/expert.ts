import { ref } from 'vue'
import { SettingsService } from '../bindings/github.com/toblu96/taktwin/internal/simapp'

// Expert mode: a teacher's way into building stations and writing exercises, switched on in the
// settings. It only shows more; it secures nothing.
export const expert = ref(false)

// Settles once the setting is read, which the pages of an open project wait for.
export const expertRead = SettingsService.ExpertMode()
  .then((on) => (expert.value = on))
  .catch(() => (expert.value = false))

export async function setExpert(on: boolean) {
  await SettingsService.SetExpertMode(on)
  expert.value = on
}
