import { shallowRef } from 'vue'
import { BuilderService, type WiringBox, type WiringState } from '../bindings/github.com/toblu96/taktwin/internal/simapp'

// The wiring of an open project, for the wiring view: as the station runs, to look at, while
// editing comes from the builder (builder.ts), which can change it.
export const stationWiring = shallowRef<WiringState | null>(null)
export const stationWiringProblem = shallowRef('')

export async function loadStationWiring() {
  try {
    stationWiring.value = await BuilderService.StationWiring()
    stationWiringProblem.value = ''
  } catch (error) {
    stationWiring.value = null
    stationWiringProblem.value = (error as Error).message
  }
}

// What a wiring says about its connectors: the one each is cabled to, how each is called, the
// colour of each kind, and the box each is on.
export function wiringIndex(w: WiringState | null) {
  const partner = new Map<string, string>()
  for (const c of w?.cables ?? []) {
    partner.set(c.from, c.to)
    partner.set(c.to, c.from)
  }
  const name = new Map<string, string>()
  const color = new Map<string, string>()
  const boxOf = new Map<string, WiringBox>()
  const boxes: WiringBox[] = []
  for (const s of w?.stations ?? []) {
    for (const b of s.boxes ?? []) {
      boxes.push(b)
      for (const c of b.connectors ?? []) {
        name.set(c.ref, `${b.of ? `${b.of} · ` : ''}${b.name} ${c.id}`)
        if (c.color) color.set(c.type, c.color)
        boxOf.set(c.ref, b)
      }
    }
  }
  return { partner, name, color, boxOf, boxes }
}

// A pin's class: only pins of the same class swap, an input with an input.
export const pinClass = (pin: string) =>
  pin.startsWith('AI') ? 'analogue input' : pin.startsWith('AQ') ? 'analogue output' : pin.startsWith('I') ? 'input' : pin.startsWith('Q') ? 'output' : pin

export const KINDS: Record<string, string> = { plc: 'PLC', interface: 'Interface', terminal: 'I/O terminal', console: 'Console' }
