import type { Signal } from '../ui/UiIoList.vue'

// Made-up signals for the lab's I/O list, shaped like the app's.
export const signals: Signal[] = [
  { key: 'b1', address: '%IX0.0', name: 'B1', description: 'Pallet at the start of the conveyor' },
  { key: 'b2', address: '%IX0.1', name: 'B2', description: 'Pallet at the stop' },
  { key: 'b3', address: '%IX0.2', name: 'B3', description: 'Pallet at the end' },
  { key: 'b4', address: '%IX0.3', name: 'B4', description: 'Magazine has pallets' },
  { key: 's1', address: '%IX0.4', name: 'S1', description: 'Start button' },
  { key: 's2', address: '%IX0.5', name: 'S2', description: 'Stop button' },
]
