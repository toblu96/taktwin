import { ref, watch } from 'vue'
import { project, simulation } from './sim'

// What happened while the station ran, for the panel below the 3D view: what ran into what, and
// workpieces that fell off, each once, at the simulated time it happened. A restart of the
// station is noted too. The log starts again with each project.
export type SimEvent = { time: number; text: string; kind: 'hit' | 'fell' | 'restart' }
export const events = ref<SimEvent[]>([])

export function clearEvents() {
  events.value = []
}

export function trackEvents() {
  let hits = 0
  let fell = 0
  let time = 0
  const add = (e: SimEvent) => (events.value = [...events.value, e])
  watch(
    () => project.value?.name,
    () => {
      clearEvents()
      hits = fell = time = 0
    },
  )
  watch(simulation.snapshot, (s) => {
    if (!s) return
    if (s.time < time) {
      hits = fell = 0
      if (events.value.length) add({ time: s.time, text: 'The station restarted', kind: 'restart' })
    }
    time = s.time
    const all = s.hits ?? []
    if (all.length < hits) hits = 0
    for (const text of all.slice(hits)) add({ time: s.time, text, kind: 'hit' })
    hits = all.length
    if (s.fellOff > fell) {
      const n = s.fellOff - fell
      add({ time: s.time, text: n === 1 ? 'A workpiece fell off the station' : `${n} workpieces fell off the station`, kind: 'fell' })
    }
    fell = s.fellOff
  })
}
