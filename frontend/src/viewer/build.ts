import * as THREE from 'three'
import type { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import type { SceneInstance } from '../../bindings/github.com/toblu96/taktwin/internal/model'

// The builder's part of the 3D view, following how CAD programs work with a mouse or a
// finger: a click only selects, a selected module moves by its arrow handles, and a module
// being added follows the pointer as a ghost until it is put down.

export interface XYZ {
  x: number
  y: number
  z: number
}

// A place a module can be screwed on, in station coordinates. Along counts from `from` in
// the direction of `dir`, between min and max, except where its rail is cut. Turn is the
// site's own turn.
export interface SiteLine {
  key: string
  type: string
  from: XYZ
  dir: XYZ
  min: number
  max: number
  turn: number
  gaps: { from: number; to: number }[]
  current: boolean
}

// Which way a handle moves a module: along its line, across to the next, or both at once.
type Axis = 'along' | 'across' | 'free'

// The selected module's handles: along its line from `at`, and across to the other lines
// of the same site, such as the plate's other slots. What is screwed onto it goes along.
export interface Handle {
  id: string
  at: number
  line: SiteLine
  lines: SiteLine[]
  carry: string[]
  // For a stand: whether a plus beside it on the left (towards -x) and right adds another.
  sides?: { left: boolean; right: boolean }
}

// Where workpieces travel and the special positions of the modules. Those of the focused
// module stand out.
export interface Overlay {
  paths: { owner: string; points: XYZ[] }[]
  marks: { owner: string; label: string; at: XYZ }[]
  focus: string | null
}

// Where the ghost of a module being added stands, and whether it fits there: true, false,
// or null while that is not known yet.
export interface GhostPose {
  at: XYZ
  rotZ: number
  ok: boolean | null
}

export interface BuildEvents {
  // The pointer is over a place for the module being added.
  hover?(key: string, along: number): void
  // A place was clicked for it. A tap on a touchscreen only shows it there, to confirm.
  place?(key: string, along: number, touch: boolean): void
  // A handle was dragged: to that many mm along a line, and let go when done.
  handle?(key: string, along: number, done: boolean): void
  // A handle was tapped: one step along or across.
  nudge?(axis: 'along' | 'across', sign: number): void
  // A plus beside the selected stand was clicked: another one on that side, -1 or 1.
  addBeside?(side: number): void
}

// What the build layer needs of the scene it is part of.
export interface BuildHost {
  root: THREE.Group
  camera: THREE.PerspectiveCamera
  controls: OrbitControls
  canvas: HTMLCanvasElement
  raycaster: THREE.Raycaster
  instances: Map<string, THREE.Group>
  invalidate(): void
  moduleBox(id: string): THREE.Box3 | null
  pick(event: { clientX: number; clientY: number }): {
    instance: string | null
  }
  buildInto(instance: SceneInstance, parent: THREE.Object3D): void
}

// Moving snaps to this many mm, and to whole mm with Shift held.
export const GRID = 5
export const snap = (along: number, fine: boolean) => (fine ? Math.round(along) : Math.round(along / GRID) * GRID)

const PLACE_REACH = 80 // mm from a site line within which the ghost snaps onto it
const POINT_REACH = 200 // the same for a site that is a single spot, such as the PLC's place in a stand
const HANDLE_SIZE = 0.03 // of the distance to the camera, so handles keep their size on screen
const ALONG_COLOR = 0xffa000
const ACROSS_COLOR = 0x2fb7a4
const SITE_COLOR = 0x2fb7a4
const PATH_COLOR = 0x4aa3ff
const HOVER_COLOR = 0x9fb3bb
const FITS_COLOR = 0x3cc97a
const REFUSED_COLOR = 0xe5484d
const UNKNOWN_COLOR = 0xc9d3d8

const vec = (p: XYZ) => new THREE.Vector3(p.x, p.y, p.z)

export class BuildLayer {
  private readonly overlay = new THREE.Group()
  private readonly siteLines = new THREE.Group()
  private readonly handles = new THREE.Group()
  private readonly ghost = new THREE.Group()
  private readonly pluses = new THREE.Group()
  private pressedPlus: number | null = null
  private readonly ghostMaterial = new THREE.MeshBasicMaterial({
    color: UNKNOWN_COLOR,
    transparent: true,
    opacity: 0.3,
    depthWrite: false,
  })
  private placing: SiteLine[] = []
  private handle: Handle | null = null
  private hovered: THREE.Box3Helper | null = null
  private hoveredId: string | null = null
  private dragging: {
    axis: Axis
    sign: number
    offset: number
    grabbed: number
    key: string
    along: number
    moved: boolean
    origins: Map<THREE.Object3D, THREE.Vector3>
  } | null = null

  constructor(
    private readonly host: BuildHost,
    private readonly events: BuildEvents,
  ) {
    host.root.add(this.overlay, this.siteLines, this.handles, this.ghost, this.pluses)
    this.ghost.visible = false
  }

  setOverlay(overlay: Overlay) {
    clear(this.overlay)
    for (const path of overlay.paths) {
      if (path.points.length < 2) continue
      const focused = path.owner === overlay.focus
      const line = new THREE.Line(
        new THREE.BufferGeometry().setFromPoints(path.points.map((p) => vec(p).add(new THREE.Vector3(0, 0, 1)))),
        new THREE.LineBasicMaterial({
          color: focused ? ALONG_COLOR : PATH_COLOR,
          transparent: true,
          opacity: focused ? 1 : 0.55,
          depthTest: false,
        }),
      )
      line.renderOrder = 9
      this.overlay.add(line)
    }
    for (const mark of overlay.marks) {
      const focused = mark.owner === overlay.focus
      const dot = new THREE.Mesh(
        new THREE.OctahedronGeometry(focused ? 6 : 4),
        new THREE.MeshBasicMaterial({
          color: focused ? ALONG_COLOR : PATH_COLOR,
          depthTest: false,
        }),
      )
      dot.position.copy(vec(mark.at)).add(new THREE.Vector3(0, 0, 2))
      dot.renderOrder = 10
      this.overlay.add(dot)
    }
    this.host.invalidate()
  }

  // The places a module being added could go, or null when nothing is being added.
  setPlacing(lines: SiteLine[] | null) {
    this.placing = lines ?? []
    this.drawLines(this.placing)
    if (!lines) this.ghost.visible = false
  }

  setGhost(instances: SceneInstance[] | null) {
    clear(this.ghost)
    if (!instances) {
      this.ghost.visible = false
      this.host.invalidate()
      return
    }
    for (const instance of instances) this.host.buildInto(instance, this.ghost)
    this.ghost.traverse((o) => {
      if (o instanceof THREE.Mesh) {
        o.material = this.ghostMaterial
        o.renderOrder = 8
      }
      o.userData.noPick = true
    })
    this.host.invalidate()
  }

  poseGhost(pose: GhostPose | null) {
    this.ghost.visible = !!pose
    if (pose) {
      this.ghost.position.copy(vec(pose.at))
      this.ghost.rotation.set(0, 0, THREE.MathUtils.degToRad(pose.rotZ))
      this.ghostMaterial.color.setHex(pose.ok === null ? UNKNOWN_COLOR : pose.ok ? FITS_COLOR : REFUSED_COLOR)
    }
    this.host.invalidate()
  }

  setHandle(handle: Handle | null) {
    this.handle = handle
    clear(this.handles)
    clear(this.pluses)
    this.drawLines(this.placing)
    if (!handle) {
      this.host.invalidate()
      return
    }
    // Right after the scene is built, its matrices are not worked out yet.
    this.host.root.updateMatrixWorld(true)
    const box = this.host.moduleBox(handle.id)
    if (!box) return
    // On top of the module, in station coordinates.
    const top = box.getCenter(new THREE.Vector3())
    top.y = box.max.y
    this.handles.position.copy(this.host.root.worldToLocal(top))
    const along = vec(handle.line.dir).normalize()
    this.addArrow(along, 'along', 1)
    this.addArrow(along.clone().negate(), 'along', -1)
    const next = this.neighbour(handle, 1) ?? this.neighbour(handle, -1)
    if (next) {
      // Towards the next slot, whichever side that is.
      const sign = this.neighbour(handle, 1) ? 1 : -1
      const across = vec(next.from).sub(vec(handle.line.from))
      across.addScaledVector(along, -across.dot(along)).normalize().multiplyScalar(sign)
      if (this.neighbour(handle, 1)) this.addArrow(across, 'across', 1)
      if (this.neighbour(handle, -1)) this.addArrow(across.clone().negate(), 'across', -1)
    }
    // The dot in the middle moves it both ways at once: along its line, and onto whichever
    // of the other lines the pointer is nearest.
    const center = new THREE.Mesh(new THREE.SphereGeometry(0.34, 16, 12), new THREE.MeshBasicMaterial({ color: 0xffffff, depthTest: false }))
    center.renderOrder = 12
    center.userData.handle = { axis: 'free', sign: 0 }
    const target = new THREE.Mesh(new THREE.SphereGeometry(0.8, 8, 6), new THREE.MeshBasicMaterial({ visible: false }))
    target.userData.handle = { axis: 'free', sign: 0 }
    this.handles.add(center, target)
    if (handle.sides) this.addPluses(box, handle.sides)
    this.host.invalidate()
  }

  // A plus on each free side of a stand, at the height of its plate, a little off its edge.
  private addPluses(box: THREE.Box3, sides: { left: boolean; right: boolean }) {
    let lo = Infinity
    let hi = -Infinity
    let top = -Infinity
    for (let k = 0; k < 8; k++) {
      const corner = new THREE.Vector3(k & 1 ? box.max.x : box.min.x, k & 2 ? box.max.y : box.min.y, k & 4 ? box.max.z : box.min.z)
      const local = this.host.root.worldToLocal(corner)
      lo = Math.min(lo, local.x)
      hi = Math.max(hi, local.x)
      top = Math.max(top, local.z)
    }
    const middle = this.host.root.worldToLocal(box.getCenter(new THREE.Vector3()))
    for (const [side, x] of [
      [-1, lo - 90],
      [1, hi + 90],
    ] as const) {
      if ((side < 0 && !sides.left) || (side > 0 && !sides.right)) continue
      const plus = new THREE.Sprite(plusMaterial())
      plus.position.set(x, middle.y, top - 40)
      plus.scale.setScalar(0.05)
      plus.renderOrder = 12
      plus.userData.plus = side
      this.pluses.add(plus)
    }
  }

  private plusAt(event: { clientX: number; clientY: number }): number | null {
    if (!this.pluses.children.length) return null
    this.ray(event)
    const hit = this.host.raycaster.intersectObject(this.pluses, true)[0]
    return hit ? (hit.object.userData.plus as number) : null
  }

  // Keeps the handles the same size on screen however near the camera is.
  beforeRender() {
    if (!this.handles.children.length) return
    const at = this.handles.getWorldPosition(new THREE.Vector3())
    this.handles.scale.setScalar(this.host.camera.position.distanceTo(at) * HANDLE_SIZE)
  }

  pointerDown(event: PointerEvent): boolean {
    this.pressedPlus = this.plusAt(event)
    if (this.pressedPlus !== null) return true
    const hit = this.handleAt(event)
    if (!hit || !this.handle) return false
    const handle = this.handle
    const moving = [handle.id, ...handle.carry]
    const origins = new Map<THREE.Object3D, THREE.Vector3>()
    for (const [name, group] of this.host.instances) {
      if (moving.some((id) => name === id || name.startsWith(`${id}/`))) origins.set(group, group.position.clone())
    }
    origins.set(this.handles, this.handles.position.clone())
    const near = this.nearLine(event, handle.line)?.along ?? handle.at
    const grabbed = hit.axis === 'along' ? near : this.nearest(event, handle.lines)
    const offset = handle.at - (hit.axis === 'across' ? grabbed : near)
    this.dragging = {
      axis: hit.axis,
      sign: hit.sign,
      offset,
      grabbed,
      key: handle.line.key,
      along: handle.at,
      moved: false,
      origins,
    }
    this.drawLines(handle.lines)
    this.host.controls.enabled = false
    return true
  }

  pointerMove(event: PointerEvent): boolean {
    const drag = this.dragging
    if (drag && this.handle) {
      const handle = this.handle
      let line = handle.line
      let along = handle.at
      if (drag.axis !== 'along') {
        // As many slots as the pointer crossed since it grabbed the handle.
        const current = handle.lines.findIndex((l) => l.key === handle.line.key)
        const index = Math.min(handle.lines.length - 1, Math.max(0, current + this.nearest(event, handle.lines) - drag.grabbed))
        line = handle.lines[index]
        along = Math.min(line.max, Math.max(line.min, handle.at))
      }
      if (drag.axis !== 'across') {
        const near = this.nearLine(event, line)
        if (!near) return true
        along = Math.min(line.max, Math.max(line.min, snap(near.along + drag.offset, event.shiftKey)))
      }
      if (along === drag.along && line.key === drag.key) return true
      drag.along = along
      drag.key = line.key
      drag.moved = true
      const shift = vec(line.from)
        .addScaledVector(vec(line.dir), along)
        .sub(vec(handle.line.from).addScaledVector(vec(handle.line.dir), handle.at))
      for (const [group, origin] of drag.origins) group.position.copy(origin).add(shift)
      this.host.invalidate()
      this.events.handle?.(line.key, along, false)
      return true
    }
    if (this.placing.length) {
      if (event.buttons) return false
      const target = this.placeAt(event)
      if (target) this.events.hover?.(target.key, target.along)
      return false
    }
    if (!event.buttons) this.hover(this.host.pick(event).instance)
    return false
  }

  pointerUp(event: PointerEvent, clicked: boolean): boolean {
    const pressed = this.pressedPlus
    if (pressed !== null) {
      this.pressedPlus = null
      if (this.plusAt(event) === pressed) this.events.addBeside?.(pressed)
      return true
    }
    const drag = this.dragging
    if (drag) {
      this.dragging = null
      this.host.controls.enabled = true
      this.drawLines(this.placing)
      if (drag.moved) this.events.handle?.(drag.key, drag.along, true)
      else if (drag.axis !== 'free') this.events.nudge?.(drag.axis, drag.sign)
      return true
    }
    if (this.placing.length && clicked) {
      const target = this.placeAt(event)
      if (target) this.events.place?.(target.key, target.along, event.pointerType === 'touch')
      return true
    }
    return false
  }

  dispose() {
    for (const group of [this.overlay, this.siteLines, this.handles, this.ghost, this.pluses]) clear(group)
    this.ghostMaterial.dispose()
  }

  // The lines of a site next to the handle's, one slot further either way.
  private neighbour(handle: Handle, sign: number) {
    const index = handle.lines.findIndex((l) => l.key === handle.line.key)
    return handle.lines[index + sign] ?? null
  }

  private addArrow(dir: THREE.Vector3, axis: 'along' | 'across', sign: number) {
    const color = axis === 'along' ? ALONG_COLOR : ACROSS_COLOR
    const material = new THREE.MeshBasicMaterial({ color, depthTest: false })
    const arrow = new THREE.Group()
    const shaft = new THREE.Mesh(new THREE.CylinderGeometry(0.07, 0.07, 1.3, 8), material)
    shaft.position.y = 1.05
    const tip = new THREE.Mesh(new THREE.ConeGeometry(0.3, 0.75, 16), material)
    tip.position.y = 2.05
    // A larger, invisible target, so a finger finds it too.
    const target = new THREE.Mesh(new THREE.SphereGeometry(0.75, 8, 6), new THREE.MeshBasicMaterial({ visible: false }))
    target.position.y = 1.8
    for (const part of [shaft, tip, target]) {
      part.renderOrder = 12
      part.userData.handle = { axis, sign }
      arrow.add(part)
    }
    arrow.quaternion.setFromUnitVectors(new THREE.Vector3(0, 1, 0), dir)
    this.handles.add(arrow)
  }

  private drawLines(lines: SiteLine[]) {
    clear(this.siteLines)
    for (const line of lines) {
      const from = vec(line.from)
      const dir = vec(line.dir)
      // A rail cut for a slide has no line where the gap is.
      const points: THREE.Vector3[] = []
      let start = line.min
      for (const gap of [...line.gaps].sort((a, b) => a.from - b.from)) {
        if (gap.from > start) points.push(at(from, dir, start), at(from, dir, gap.from))
        start = Math.max(start, gap.to)
      }
      if (start < line.max) points.push(at(from, dir, start), at(from, dir, line.max))
      // A site that is a single spot shows as a cross.
      if (line.max - line.min < 1e-6) {
        const spot = at(from, dir, line.min)
        for (const axis of [new THREE.Vector3(30, 0, 0), new THREE.Vector3(0, 30, 0), new THREE.Vector3(0, 0, 30)]) {
          points.push(spot.clone().sub(axis), spot.clone().add(axis))
        }
      }
      const geometry = new THREE.BufferGeometry().setFromPoints(points)
      const drawn = new THREE.LineSegments(
        geometry,
        new THREE.LineBasicMaterial({
          color: line.current ? ALONG_COLOR : SITE_COLOR,
          depthTest: false,
        }),
      )
      drawn.renderOrder = 9
      this.siteLines.add(drawn)
    }
    this.host.invalidate()
  }

  private hover(id: string | null) {
    if (id === this.hoveredId) return
    this.hoveredId = id
    if (this.hovered) {
      this.hovered.parent?.remove(this.hovered)
      this.hovered.dispose()
      this.hovered = null
    }
    const box = id ? this.host.moduleBox(id) : null
    if (box) {
      this.hovered = new THREE.Box3Helper(box, HOVER_COLOR)
      this.host.root.parent?.add(this.hovered)
    }
    this.host.invalidate()
  }

  private ray(event: { clientX: number; clientY: number }) {
    const rect = this.host.canvas.getBoundingClientRect()
    const pointer = new THREE.Vector2(((event.clientX - rect.left) / rect.width) * 2 - 1, -((event.clientY - rect.top) / rect.height) * 2 + 1)
    this.host.raycaster.setFromCamera(pointer, this.host.camera)
    this.host.root.updateMatrixWorld()
    const toStation = this.host.root.matrixWorld.clone().invert()
    return {
      origin: this.host.raycaster.ray.origin.clone().applyMatrix4(toStation),
      dir: this.host.raycaster.ray.direction.clone().transformDirection(toStation),
    }
  }

  private handleAt(event: { clientX: number; clientY: number }): { axis: Axis; sign: number } | null {
    if (!this.handles.children.length) return null
    this.ray(event)
    // The arrows before the dot's larger target, which they reach into.
    const hits = this.host.raycaster.intersectObject(this.handles, true).filter((h) => h.object.userData.handle)
    const hit = hits.find((h) => h.object.userData.handle.axis !== 'free') ?? hits[0]
    return hit ? (hit.object.userData.handle as { axis: Axis; sign: number }) : null
  }

  // Where the pointer's ray passes closest to a line: mm along it, and how far apart.
  private nearLine(event: { clientX: number; clientY: number }, line: SiteLine) {
    const { origin, dir: ray } = this.ray(event)
    const from = vec(line.from)
    const dir = vec(line.dir)
    const w = from.clone().sub(origin)
    const b = dir.dot(ray)
    const denom = 1 - b * b
    if (denom < 1e-6) return null
    const t = Math.min(line.max, Math.max(line.min, (b * ray.dot(w) - dir.dot(w)) / denom))
    const point = from.addScaledVector(dir, t)
    const onRay = origin.clone().addScaledVector(ray, Math.max(0, ray.dot(point.clone().sub(origin))))
    return { along: t, distance: point.distanceTo(onRay) }
  }

  // Which of the lines the pointer is nearest.
  private nearest(event: { clientX: number; clientY: number }, lines: SiteLine[]) {
    let index = 0
    let distance = Infinity
    lines.forEach((line, i) => {
      const near = this.nearLine(event, line)
      if (near && near.distance < distance) [index, distance] = [i, near.distance]
    })
    return index
  }

  // The place under the pointer for the module being added, on the grid.
  private placeAt(event: PointerEvent) {
    let best: { line: SiteLine; along: number; distance: number } | null = null
    for (const line of this.placing) {
      const near = this.nearLine(event, line)
      const reach = line.max - line.min < 1e-6 ? POINT_REACH : PLACE_REACH
      if (near && near.distance < reach && (!best || near.distance < best.distance)) best = { line, ...near }
    }
    if (!best) return null
    return {
      key: best.line.key,
      along: Math.min(best.line.max, Math.max(best.line.min, snap(best.along, event.shiftKey))),
    }
  }
}

// The plus that adds a stand: a round teal button, the same size on screen at any distance.
let plusMaterialCache: THREE.SpriteMaterial | null = null
function plusMaterial() {
  if (plusMaterialCache) return plusMaterialCache
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = 128
  const g = canvas.getContext('2d')!
  g.fillStyle = '#2fb7a4'
  g.beginPath()
  g.arc(64, 64, 60, 0, Math.PI * 2)
  g.fill()
  g.fillStyle = '#ffffff'
  g.fillRect(58, 30, 12, 68)
  g.fillRect(30, 58, 68, 12)
  plusMaterialCache = new THREE.SpriteMaterial({
    map: new THREE.CanvasTexture(canvas),
    depthTest: false,
    sizeAttenuation: false,
  })
  return plusMaterialCache
}

function clear(group: THREE.Object3D) {
  for (const child of [...group.children]) {
    group.remove(child)
    child.traverse((o) => {
      if (o instanceof THREE.Mesh || o instanceof THREE.Line) o.geometry.dispose()
    })
  }
}

// The point a distance along a site line, just above it.
function at(from: THREE.Vector3, dir: THREE.Vector3, along: number) {
  return from
    .clone()
    .addScaledVector(dir, along)
    .add(new THREE.Vector3(0, 0, 0.6))
}
