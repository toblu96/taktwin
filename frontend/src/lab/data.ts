import type { Signal } from '../ui/UiIoList.vue'

// Made-up data for the lab's layouts, shaped like the app's.
export const signals: Signal[] = [
  { key: 'b1', address: '%IX0.0', name: 'B1', description: 'Pallet at the start of the conveyor' },
  { key: 'b2', address: '%IX0.1', name: 'B2', description: 'Pallet at the stop' },
  { key: 'b3', address: '%IX0.2', name: 'B3', description: 'Pallet at the end' },
  { key: 'b4', address: '%IX0.3', name: 'B4', description: 'Magazine has pallets' },
  { key: 's1', address: '%IX0.4', name: 'S1', description: 'Start button' },
  { key: 's2', address: '%IX0.5', name: 'S2', description: 'Stop button' },
]

export const projects = [
  { name: 'Load a pallet', what: 'Exercise · Pallet loading', when: '2 hours ago', picture: '/lab/exercise.jpg', exercise: true, check: true },
  { name: 'Pallet loading', what: 'Pallet loading', when: 'yesterday', picture: '/lab/station.jpg', exercise: false, check: undefined },
  { name: 'My station', what: 'Own station', when: '3 days ago', picture: '/lab/empty.jpg', exercise: false, check: undefined },
  { name: 'Sorting by colour', what: 'Exercise · Pallet loading', when: 'last week', picture: '/lab/station.jpg', exercise: true, check: false },
]

export const modules = [
  { id: 'conveyor', name: 'Pallet conveyor', children: ['B1 Pallet at start', 'B2 Pallet at the stop', 'B3 Pallet at end', 'Pallet stop'] },
  { id: 'magazine', name: 'Pallet magazine', children: ['I/O block'] },
  { id: 'cartons', name: 'Carton conveyor', children: ['B1 Carton at start', 'B2 Carton at the pick place'] },
  { id: 'gantry', name: 'Gantry', children: ['X axis', 'Y axis', 'Z axis', 'Gripper'] },
  { id: 'cabinet', name: 'Control cabinet', children: [] },
]

export const checks = [
  { ok: true, line: 'The start lamp asks for Start' },
  { ok: true, line: 'The station is in its base position' },
  { ok: true, line: 'A pallet comes out of the magazine onto the conveyor' },
  { ok: true, line: 'The pallet runs to the stop' },
  { ok: false, line: 'The gantry takes the carton off the belt (after 40 s)' },
]
