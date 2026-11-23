import { ref } from 'vue'
import { Service } from '../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { OriginalInfo } from '../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import { ask, choose } from './ask'
import { connectionsChanged, editingConnection, project, refreshProject, simulation } from './sim'

// The open project's original, which every project keeps: when it was set and which parts
// differ from it. A part is put back as the original has it, or everything at once; in expert
// mode, the project as it is now becomes the original.
export const original = ref<OriginalInfo | null>(null)

export async function refreshOriginal() {
  original.value = project.value ? await Service.Original() : null
}

// when says when the original was set, for the questions below.
const when = () => (original.value?.at ? ` on ${new Date(original.value.at).toLocaleString()}` : '')

// Puts a part back, by its name in the question, after asking. A changed station loads again.
export async function putBack(part: 'connection' | 'programs' | 'station' | 'settings' | 'everything', plc: string, what: string) {
  const ok = await ask(`Put back ${what} as it was in the project's original${when()}? What was changed in it since is lost.`, 'Put back')
  if (!ok) return
  original.value = await Service.PutBack(part, plc)
  if (part === 'station' || part === 'settings' || part === 'everything') simulation.scene.value = await Service.Scene()
  if (part === 'connection' || part === 'everything') {
    // The bindings open in the wiring are those that were put back.
    editingConnection.value = null
    connectionsChanged.value++
  }
  await refreshProject()
}

// Makes the project as it is now its original, after asking.
export async function keepAsOriginal() {
  const ok = await ask(
    `Keep ${project.value?.name ?? 'the project'} as it is now as its original? Putting back then goes back to this state, not to the original${when()}.`,
    'Keep as original',
  )
  if (ok) original.value = await Service.KeepAsOriginal()
}

// Hands a project out as a copy for others, whose original is the state it is handed out in,
// with the programs only when asked, so that a solution does not go out by mistake.
export async function handOut(p: { name: string; path: string; exercise?: string; exerciseName?: string }) {
  const WITH = 'With the programs'
  const answer = await choose(
    `Hand out ${p.exerciseName || p.name}? Others get a copy of it as it is now, with its station, how its PLCs are connected and the collections it carries, and can go back to this state. Should the programs go along?`,
    ['Without programs', WITH],
  )
  if (answer) return Service.HandOut(p.path, answer === WITH)
}
