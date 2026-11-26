import { ref, watch } from 'vue'
import { project, simulation } from './sim'

// What happened while the station ran, for the panel below the 3D view: what ran into what,
// workpieces that fell off, and how each PLC's connection went, each once, at the time of day
// it happened, as a log has it: the simulated time starts again with each restart. A restart of
// the station is noted too. The log starts again with each project.
// Time is when it happened, in milliseconds since 1970; detail a second line, such as why a
// connection failed.
export type SimEvent = { time: number; text: string; kind: 'hit' | 'fell' | 'restart' | 'connected' | 'lost' | 'disconnected'; detail?: string }

// How a PLC's connection stood at the last snapshot: its state, "" for none, and whether it was
// connected or has failed since it last connected, so that a link that keeps retrying is
// reported once.
export type LinkMemo = { state: string; endpoint: string; wasConnected: boolean; failed: boolean }
export const noLink: LinkMemo = { state: '', endpoint: '', wasConnected: false, failed: false }

// What a change of a PLC's connection says, if anything, and how it then stands: connected, again
// after it was lost; lost, once, or could not connect, once, until it connects; disconnected.
export function linkEvent(prev: LinkMemo, link: { state: string; endpoint: string; detail?: string } | null, name: string): [Omit<SimEvent, 'time'> | null, LinkMemo] {
  const state = link?.state ?? ''
  const endpoint = link?.endpoint ?? ''
  if (state === '') return [prev.state === '' ? null : { text: `${name} disconnected`, kind: 'disconnected' }, noLink]
  if (state === 'connected' && prev.state !== 'connected') {
    const text = prev.wasConnected ? `${name} connected again to ${endpoint}` : `${name} connected to ${endpoint}`
    return [
      { text, kind: 'connected' },
      { state, endpoint, wasConnected: true, failed: false },
    ]
  }
  if (state === 'failed' && !prev.failed) {
    const to = endpoint ? ` to ${endpoint}` : ''
    const text = prev.state === 'connected' ? `${name} lost its connection${to}` : `${name} could not connect${to}`
    return [
      { text, kind: 'lost', detail: link?.detail || undefined },
      { ...prev, state, endpoint, failed: true },
    ]
  }
  return [null, { ...prev, state, endpoint: endpoint || prev.endpoint }]
}
export const events = ref<SimEvent[]>([])

export function clearEvents() {
  events.value = []
}

export function trackEvents() {
  let hits = 0
  let fell = 0
  let time = 0
  let links: LinkMemo[] = []
  const add = (e: SimEvent) => (events.value = [...events.value, e])
  const plcName = (i: number) => {
    const scene = simulation.scene.value
    const module = scene?.plcs?.[i]?.module ?? scene?.plc?.module
    return scene?.instances?.find((m) => m.id === module)?.name ?? 'The PLC'
  }
  watch(
    () => project.value?.name,
    () => {
      clearEvents()
      hits = fell = time = 0
      links = []
    },
  )
  watch(simulation.snapshot, (s) => {
    if (!s) return
    if (s.time < time) {
      hits = fell = 0
      if (events.value.length) add({ time: Date.now(), text: 'The station restarted', kind: 'restart' })
    }
    time = s.time
    const all = s.hits ?? []
    if (all.length < hits) hits = 0
    for (const text of all.slice(hits)) add({ time: Date.now(), text, kind: 'hit' })
    hits = all.length
    if (s.fellOff > fell) {
      const n = s.fellOff - fell
      add({ time: Date.now(), text: n === 1 ? 'A workpiece fell off the station' : `${n} workpieces fell off the station`, kind: 'fell' })
    }
    fell = s.fellOff
    ;(s.plcs ?? []).forEach((view, i) => {
      const [event, memo] = linkEvent(links[i] ?? noLink, view.link ?? null, plcName(i))
      links[i] = memo
      if (event) add({ time: Date.now(), ...event })
    })
  })
}
