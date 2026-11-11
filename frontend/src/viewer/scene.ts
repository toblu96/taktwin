import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js'
import type { Snapshot } from '../../bindings/github.com/toblu96/taktwin/internal/engine'
import type {
  MarkDef,
  Material,
  Point,
  Scene,
  SceneGeometry,
  SceneInstance,
  SceneLabel,
  SceneLamp,
  SceneNode,
  WorkpieceKind,
} from '../../bindings/github.com/toblu96/taktwin/internal/model'
import type { WorkpieceView } from '../../bindings/github.com/toblu96/taktwin/internal/sim'
import { BuildLayer, type BuildEvents } from './build'

const DRAG_THRESHOLD = 5
const CAMERA_FOV = 32
const SELECTION_COLOR = 0xffa000
const HOME_TARGET = new THREE.Vector3(175, 500, -330)
const HOME_POSITION = new THREE.Vector3(-134, 1860, 1124)
const FIT_DIRECTION = new THREE.Vector3(-0.45, 0.5, 0.74).normalize()
// LARGE_STATION is the size, corner to corner in mm, beyond which a station is framed as a
// whole instead of from the home view.
const LARGE_STATION = 4000

const v = (p: Point) => new THREE.Vector3(p.x, p.y, p.z)
const rad = (deg: number) => (deg * Math.PI) / 180

export interface StationEvents {
  select(instance: string | null): void
  control(name: string, down: boolean, toggle: boolean): void
  // The view changed, so anything anchored to the 3D model must follow.
  rendered(): void
  // In the builder, what the build layer reports.
  build?: BuildEvents
}

function createMaterial(m: Material): THREE.Material {
  const opacity = m.opacity ? { transparent: true, opacity: m.opacity } : {}
  if (m.type === 'MeshPhongMaterial') {
    return new THREE.MeshPhongMaterial({ color: m.color, shininess: m.shininess ?? 30, specular: m.specular ?? '#333333', ...opacity })
  }
  return new THREE.MeshLambertMaterial({ color: m.color, ...opacity })
}

interface LampRange {
  colors: THREE.BufferAttribute
  start: number
  count: number
  on: THREE.Color
  off: THREE.Color
  state: number
  shown: boolean | null
}

// Static nodes are merged per material, and plain lamps into one vertex-coloured
// mesh, to keep draw calls low on integrated GPUs.
class StaticBatch {
  private readonly groups = new Map<string, THREE.BufferGeometry[]>()
  private readonly lamps: { geometry: THREE.BufferGeometry; lamp: SceneLamp }[] = []
  private readonly glows = new Map<number, { center: THREE.Vector3; lamp: SceneLamp }[]>()
  private readonly labels: { label: SceneLabel; matrix: THREE.Matrix4 }[] = []

  add(material: string, geometry: THREE.BufferGeometry, matrix: THREE.Matrix4) {
    const list = this.groups.get(material) ?? []
    list.push(prepared(geometry, matrix))
    this.groups.set(material, list)
  }

  addLamp(lamp: SceneLamp, geometry: THREE.BufferGeometry, matrix: THREE.Matrix4) {
    const flat = prepared(geometry, matrix)
    this.lamps.push({ geometry: flat, lamp })
    this.addGlow(lamp, flat)
  }

  addLabel(label: SceneLabel, matrix: THREE.Matrix4) {
    this.labels.push({ label, matrix })
  }

  // A lit lamp gets a soft halo. One sprite batch per halo size keeps this to a
  // single draw call, however many LEDs a module has.
  addGlow(lamp: SceneLamp, placed: THREE.BufferGeometry) {
    if (!lamp.glow) return
    placed.computeBoundingSphere()
    const list = this.glows.get(lamp.glow) ?? []
    list.push({ center: placed.boundingSphere!.center.clone(), lamp })
    this.glows.set(lamp.glow, list)
  }

  build(parent: THREE.Object3D, materials: Map<string, THREE.Material>, ranges: LampRange[]) {
    for (const [material, geometries] of this.groups) {
      parent.add(new THREE.Mesh(mergeGeometries(geometries), materials.get(material)))
    }
    this.buildGlows(parent, ranges)
    this.buildLabels(parent)
    if (!this.lamps.length) return
    const merged = mergeGeometries(this.lamps.map((l) => l.geometry))
    const colors = new THREE.BufferAttribute(new Float32Array(merged.attributes.position.count * 3), 3)
    merged.setAttribute('color', colors)
    let start = 0
    for (const { geometry, lamp } of this.lamps) {
      const count = geometry.attributes.position.count
      ranges.push({ colors, start, count, on: new THREE.Color(lamp.on), off: new THREE.Color(lamp.off), state: lamp.state, shown: null })
      start += count
    }
    parent.add(new THREE.Mesh(merged, new THREE.MeshBasicMaterial({ vertexColors: true })))
  }

  // All labels of a module share one texture and one draw call.
  private buildLabels(parent: THREE.Object3D) {
    if (!this.labels.length) return
    const atlas = labelAtlas(this.labels.map((l) => l.label))
    const planes = this.labels.map(({ label, matrix }) => {
      const cell = atlas.cells.get(labelKey(label))!
      const plane = new THREE.PlaneGeometry((label.size * cell.width) / LABEL_CELL, label.size)
      const uv = plane.attributes.uv
      for (let i = 0; i < uv.count; i++) {
        uv.setXY(i, (cell.x + uv.getX(i) * cell.width) / atlas.width, 1 - (cell.y + (1 - uv.getY(i)) * LABEL_CELL) / atlas.height)
      }
      return prepared(plane, matrix)
    })
    const mesh = new THREE.Mesh(mergeGeometries(planes), new THREE.MeshBasicMaterial({ map: atlas.texture, transparent: true, depthWrite: false }))
    mesh.userData.noPick = true
    parent.add(mesh)
  }

  private buildGlows(parent: THREE.Object3D, ranges: LampRange[]) {
    for (const [size, list] of this.glows) {
      const geometry = new THREE.BufferGeometry().setFromPoints(list.map((g) => g.center))
      // Four components, so an unlit halo is fully transparent on any background.
      const colors = new THREE.BufferAttribute(new Float32Array(list.length * 4), 4)
      geometry.setAttribute('color', colors)
      list.forEach(({ lamp }, start) => {
        const on = new THREE.Color(lamp.on)
        ranges.push({ colors, start, count: 1, on, off: on, state: lamp.state, shown: null })
      })
      const material = new THREE.PointsMaterial({
        // three.js scales point sizes without the camera's field of view.
        size: size / Math.tan(rad(CAMERA_FOV) / 2),
        map: glowTexture(),
        vertexColors: true,
        transparent: true,
        depthWrite: false,
      })
      // A halo is flat and centred on its lamp, so a surface close behind the lamp
      // would cut through it and fight it for depth. Testing it one radius closer
      // to the camera keeps it whole, while nearer parts still hide it.
      material.onBeforeCompile = (shader) => {
        shader.uniforms.glowBias = { value: size / 2 }
        shader.vertexShader = shader.vertexShader.replace('void main() {', 'uniform float glowBias;\nvoid main() {').replace(
          '#include <project_vertex>',
          `#include <project_vertex>
          vec4 biased = projectionMatrix * vec4(mvPosition.xyz + normalize(-mvPosition.xyz) * glowBias, 1.0);
          gl_Position.z = biased.z / biased.w * gl_Position.w;`,
        )
      }
      const points = new THREE.Points(geometry, material)
      points.raycast = () => {}
      parent.add(points)
    }
  }
}

let sharedGlowTexture: THREE.Texture | null = null

function glowTexture() {
  if (sharedGlowTexture) return sharedGlowTexture
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = 64
  const context = canvas.getContext('2d')!
  const gradient = context.createRadialGradient(32, 32, 0, 32, 32, 32)
  gradient.addColorStop(0, 'rgba(255,255,255,0.8)')
  gradient.addColorStop(0.4, 'rgba(255,255,255,0.3)')
  gradient.addColorStop(1, 'rgba(255,255,255,0)')
  context.fillStyle = gradient
  context.fillRect(0, 0, 64, 64)
  sharedGlowTexture = new THREE.CanvasTexture(canvas)
  sharedGlowTexture.colorSpace = THREE.SRGBColorSpace
  return sharedGlowTexture
}

function prepared(geometry: THREE.BufferGeometry, matrix: THREE.Matrix4) {
  const flat = geometry.index ? geometry.toNonIndexed() : geometry
  flat.applyMatrix4(matrix)
  return flat
}

function cylinderGeometry(from: THREE.Vector3, to: THREE.Vector3, radius: number, segments: number) {
  const direction = to.clone().sub(from)
  const geometry = new THREE.CylinderGeometry(radius, radius, direction.length(), segments || (radius < 6 ? 12 : 24))
  const rotation = new THREE.Quaternion().setFromUnitVectors(new THREE.Vector3(0, 1, 0), direction.clone().normalize())
  const middle = from.clone().add(to).multiplyScalar(0.5)
  geometry.applyMatrix4(new THREE.Matrix4().compose(middle, rotation, new THREE.Vector3(1, 1, 1)))
  return geometry
}

function sectorGeometry(g: SceneGeometry) {
  const shape = new THREE.Shape()
  shape.absarc(g.center[0], g.center[1], g.outer, rad(g.start), rad(g.end), false)
  shape.absarc(g.center[0], g.center[1], g.inner, rad(g.end), rad(g.innerStart), true)
  return new THREE.ExtrudeGeometry(shape, { depth: g.height, bevelEnabled: false, curveSegments: 16 })
}

function extrudeGeometry(g: SceneGeometry) {
  const outline = (points: number[][]) => points.map(([x, y]) => new THREE.Vector2(x, y))
  const shape = new THREE.Shape(outline(g.points ?? []))
  shape.holes = (g.holes ?? []).map((hole) => new THREE.Path(outline(hole ?? [])))
  return new THREE.ExtrudeGeometry(shape, { depth: g.depth, bevelEnabled: false })
}

function createGeometry(g: SceneGeometry): THREE.BufferGeometry {
  switch (g.type) {
    case 'BoxGeometry':
      return new THREE.BoxGeometry(g.width, g.height, g.depth)
    case 'PlaneGeometry':
      return new THREE.PlaneGeometry(g.width, g.height)
    case 'CylinderGeometry':
      return cylinderGeometry(v(g.from), v(g.to), g.radius, g.radialSegments ?? 0)
    case 'ExtrudeGeometry':
      return extrudeGeometry(g)
    case 'SphereGeometry':
      return new THREE.SphereGeometry(g.radius, 12, 8)
    case 'SectorGeometry':
      return sectorGeometry(g)
  }
  throw new Error(`unknown geometry ${g.type}`)
}

function beltTexture() {
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = 64
  const context = canvas.getContext('2d')!
  context.fillStyle = '#25292c'
  context.fillRect(0, 0, 64, 64)
  context.fillStyle = '#34393d'
  context.fillRect(0, 0, 6, 64)
  const texture = new THREE.CanvasTexture(canvas)
  texture.wrapS = texture.wrapT = THREE.RepeatWrapping
  texture.colorSpace = THREE.SRGBColorSpace
  return texture
}

const LABEL_CELL = 64

const labelKey = (l: SceneLabel) => `${l.text}|${l.color ?? ''}|${l.background ?? ''}`

// Draws each different label once into a shared canvas.
function labelAtlas(labels: SceneLabel[]) {
  const font = `700 ${LABEL_CELL * 0.72}px system-ui, sans-serif`
  const canvas = document.createElement('canvas')
  const context = canvas.getContext('2d')!
  context.font = font
  const unique = new Map(labels.map((l) => [labelKey(l), l]))
  const width = 1024
  const cells = new Map<string, { x: number; y: number; width: number }>()
  let x = 0
  let y = 0
  for (const [key, label] of unique) {
    const cell = Math.min(width, Math.max(LABEL_CELL, Math.ceil(context.measureText(label.text).width) + 16))
    if (x + cell > width) {
      x = 0
      y += LABEL_CELL
    }
    cells.set(key, { x, y, width: cell })
    x += cell
  }
  canvas.width = width
  canvas.height = y + LABEL_CELL
  context.font = font
  context.textAlign = 'center'
  context.textBaseline = 'middle'
  for (const [key, label] of unique) {
    const cell = cells.get(key)!
    if (label.background) {
      context.fillStyle = label.background
      context.beginPath()
      context.roundRect(cell.x + 2, cell.y + 2, cell.width - 4, LABEL_CELL - 4, 9)
      context.fill()
    }
    context.fillStyle = label.color || '#1d2629'
    context.fillText(label.text, cell.x + cell.width / 2, cell.y + LABEL_CELL / 2 + 2)
  }
  const texture = new THREE.CanvasTexture(canvas)
  texture.colorSpace = THREE.SRGBColorSpace
  texture.anisotropy = 8
  return { texture, cells, width: canvas.width, height: canvas.height }
}

interface Moving {
  object: THREE.Object3D
  base: THREE.Matrix4
  node: SceneNode
}

export interface CameraView {
  position: number[]
  target: number[]
}

export class StationScene {
  private readonly renderer: THREE.WebGLRenderer
  private readonly scene = new THREE.Scene()
  private readonly camera = new THREE.PerspectiveCamera(CAMERA_FOV, 1, 20, 8000)
  private readonly controls: OrbitControls
  private readonly root = new THREE.Group()
  private readonly raycaster = new THREE.Raycaster()
  private readonly materials = new Map<string, THREE.Material>()
  private readonly instances = new Map<string, THREE.Group>()
  private readonly moving: Moving[] = []
  private readonly lamps: { material: THREE.MeshBasicMaterial; state: number; on: number; off: number }[] = []
  private readonly lampRanges: LampRange[] = []
  private readonly belts: { texture: THREE.Texture; state: number }[] = []
  private readonly workpieceMeshes = new Map<number, THREE.Object3D>()
  // Kinds drawn from a model, as a pallet of boards and blocks: built once, cloned for each.
  private readonly workpieceTemplates = new Map<string, THREE.Object3D>()
  private readonly workpieceGeometry = new Map<string, THREE.BufferGeometry>()
  private readonly workpieceMaterials = new Map<string, THREE.Material>()
  private readonly workpieceKinds = new Map<string, WorkpieceKind>()
  private readonly markDefs = new Map<string, MarkDef>()
  private readonly markMaterials = new Map<string, THREE.Material>()
  private readonly holeMaterial = new THREE.MeshBasicMaterial({ color: 0x111314 })
  private workpieceHeight = 0
  private workpieceFloor = 0 // where the inside of a workpiece starts, for a filling
  private workpieceInner = 0 // the radius of that inside
  private readonly beltBase = beltTexture()
  private selectionBox: THREE.Box3Helper | null = null
  private signature = ''
  private frame = 0
  private active = true
  private pressed: { name: string; toggle: boolean } | null = null
  private downAt: { x: number; y: number } | null = null
  private readonly marks = new THREE.Group()
  // In the builder: handles, ghosts and what modules line up with, instead of controls.
  readonly build: BuildLayer | null = null

  constructor(
    private readonly canvas: HTMLCanvasElement,
    data: Scene,
    private readonly events: StationEvents,
    private readonly options: { fit?: boolean; view?: CameraView; building?: boolean } = {},
  ) {
    this.renderer = new THREE.WebGLRenderer({ canvas, antialias: true, powerPreference: 'low-power' })
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
    this.scene.background = new THREE.Color(0xd5dcdc)

    for (const [name, m] of Object.entries(data.materials ?? {})) {
      if (m) this.materials.set(name, createMaterial(m))
    }

    const points = (data.workpieces.points ?? []).map(([r, z]) => new THREE.Vector2(r, z))
    const lathe = (profile: THREE.Vector2[]) => {
      const geometry = new THREE.LatheGeometry(profile, 32).rotateX(Math.PI / 2)
      geometry.computeVertexNormals()
      return geometry
    }
    const base = lathe(points)
    this.workpieceHeight = data.workpieces.height
    // The inside of a cup-shaped workpiece: the last point of its profile on the axis is its floor.
    const last = points[points.length - 1]
    this.workpieceFloor = last && last.x === 0 ? last.y : 0
    this.workpieceInner = data.workpieces.diameter / 2 - 3
    for (const mark of data.workpieces.marks ?? []) {
      this.markDefs.set(mark.id, mark)
      if (mark.material) this.markMaterials.set(mark.id, createMaterial(mark.material))
    }
    for (const kind of data.workpieces.kinds ?? []) {
      this.workpieceKinds.set(kind.id, kind)
      this.workpieceMaterials.set(kind.id, createMaterial(kind.material))
      const model = data.models?.[kind.id]
      if (model?.length) {
        const group = new THREE.Group()
        const batch = new StaticBatch()
        this.buildNodes(model, new THREE.Matrix4(), batch, group)
        batch.build(group, this.materials, this.lampRanges)
        this.workpieceTemplates.set(kind.id, group)
        continue
      }
      if (kind.size?.length === 2) {
        // A block of its size, standing on its base.
        const height = kind.height || data.workpieces.height
        this.workpieceGeometry.set(kind.id, new THREE.BoxGeometry(kind.size[0], kind.size[1], height).translate(0, 0, height / 2))
        continue
      }
      if (kind.points?.length) {
        this.workpieceGeometry.set(kind.id, lathe(kind.points.map(([r, z]) => new THREE.Vector2(r, z))))
        continue
      }
      // A kind of its own height is the same shape, with the top where it says.
      const scale = kind.height ? kind.height / data.workpieces.height : 1
      this.workpieceGeometry.set(kind.id, scale === 1 ? base : base.clone().scale(1, 1, scale))
    }

    this.scene.add(new THREE.HemisphereLight(0xffffff, 0x8a9499, 2.1))
    const key = new THREE.DirectionalLight(0xffffff, 1.7)
    key.position.set(-500, 1600, 900)
    this.scene.add(key)
    const fill = new THREE.DirectionalLight(0xffffff, 0.5)
    fill.position.set(900, 800, -900)
    this.scene.add(fill)

    // Station coordinates are z-up; rotating the root keeps three.js y-up.
    this.root.rotation.x = -Math.PI / 2
    this.scene.add(this.root)
    this.scene.add(this.marks)
    for (const instance of data.instances ?? []) this.buildInstance(instance)
    const mounted = new Set((data.instances ?? []).filter((instance) => instance.mountedOn).map((instance) => instance.id))
    for (const instance of data.instances ?? []) this.mount(instance, mounted)

    this.controls = new OrbitControls(this.camera, canvas)
    this.controls.maxPolarAngle = Math.PI * 0.62
    this.controls.minDistance = 120
    this.controls.maxDistance = 4000
    this.home(true)
    this.controls.addEventListener('change', () => this.invalidate())
    if (options.building) {
      this.build = new BuildLayer(
        {
          root: this.root,
          camera: this.camera,
          controls: this.controls,
          canvas,
          raycaster: this.raycaster,
          instances: this.instances,
          invalidate: () => this.invalidate(),
          moduleBox: (id) => this.moduleBox(id),
          pick: (event) => this.pick(event),
          buildInto: (instance, parent) => this.buildInstance(instance, parent),
        },
        events.build ?? {},
      )
    }

    this.invalidate()

    // Capture so a press on a control can disable orbiting before it starts.
    canvas.addEventListener('pointerdown', this.onPointerDown, { capture: true })
    canvas.addEventListener('pointermove', this.onPointerMove)
    canvas.addEventListener('dblclick', this.onDoubleClick)
    window.addEventListener('pointerup', this.onPointerUp)
    window.addEventListener('pointercancel', this.onPointerUp)
  }

  // Builds a module into the station, or into another group, such as the builder's ghost.
  private buildInstance(instance: SceneInstance, parent?: THREE.Object3D) {
    const group = new THREE.Group()
    group.position.copy(v(instance.at))
    group.rotation.order = 'ZYX'
    group.rotation.x = rad(instance.rotateX)
    group.rotation.y = rad(instance.rotateY)
    group.rotation.z = rad(instance.rotateZ)
    group.userData.instance = instance.selectAs || instance.id
    ;(parent ?? this.root).add(group)
    if (!parent) this.instances.set(instance.id, group)
    const batch = new StaticBatch()
    this.buildNodes(instance.nodes ?? [], new THREE.Matrix4(), batch, group)
    batch.build(group, this.materials, this.lampRanges)
  }

  // Puts a module on the moving node of the one around it, where it stands at rest.
  private mount(instance: SceneInstance, mounted: Set<string>) {
    if (!instance.mountedOn || !instance.parent) return
    let node: THREE.Object3D | undefined
    this.instances.get(instance.parent)?.traverse((o) => {
      if (o.userData.node === instance.mountedOn) node = o
    })
    const group = this.instances.get(instance.id)
    if (!group || !node) return
    node.attach(group)
    // Its own parts, such as the reed switches of a cylinder, ride along, unless they ride
    // on a node of their own.
    const prefix = instance.id + '/'
    for (const [id, part] of this.instances) {
      if (id.startsWith(prefix) && part.parent === this.root && !mounted.has(id)) group.attach(part)
    }
  }

  private buildNodes(nodes: SceneNode[], parent: THREE.Matrix4, batch: StaticBatch, object: THREE.Object3D) {
    for (const node of nodes) {
      const rotation = new THREE.Euler(rad(node.rotation.x), rad(node.rotation.y), rad(node.rotation.z))
      const matrix = parent
        .clone()
        .multiply(new THREE.Matrix4().makeTranslation(node.position.x, node.position.y, node.position.z))
        .multiply(new THREE.Matrix4().makeRotationFromEuler(rotation))

      if (node.joint) {
        // Everything under a joint moves together, in its own merged group.
        const moving = new THREE.Group()
        moving.matrixAutoUpdate = false
        moving.matrix.copy(matrix)
        // Anything attached to a pressable part presses it too, like a key's handle.
        moving.userData.press = node.press
        moving.userData.toggle = node.toggle
        moving.userData.node = node.name
        object.add(moving)
        this.moving.push({ object: moving, base: matrix, node })
        const inner = new StaticBatch()
        this.buildLeaf(node, new THREE.Matrix4(), inner, moving)
        this.buildNodes(node.children ?? [], new THREE.Matrix4(), inner, moving)
        inner.build(moving, this.materials, this.lampRanges)
        continue
      }
      this.buildLeaf(node, matrix, batch, object)
      this.buildNodes(node.children ?? [], matrix, batch, object)
    }
  }

  private buildLeaf(node: SceneNode, matrix: THREE.Matrix4, batch: StaticBatch, object: THREE.Object3D) {
    if (node.label) {
      batch.addLabel(node.label, matrix)
      return
    }
    if (!node.geometry) return
    const geometry = createGeometry(node.geometry)

    if (node.belt >= 0) {
      const texture = this.beltBase.clone()
      texture.repeat.set(node.geometry.width / 40, 1)
      const mesh = new THREE.Mesh(geometry, new THREE.MeshLambertMaterial({ map: texture }))
      mesh.applyMatrix4(matrix)
      object.add(mesh)
      this.belts.push({ texture, state: node.belt })
      return
    }
    if (!node.lamp && !node.press && !node.noPick) {
      batch.add(node.material ?? '', geometry, matrix)
      return
    }
    if (node.lamp && !node.press && !node.lamp.opacity && !node.noPick) {
      batch.addLamp(node.lamp, geometry, matrix)
      return
    }
    let material = this.materials.get(node.material ?? '')
    if (node.lamp) {
      batch.addGlow(node.lamp, prepared(geometry.clone(), matrix))
      const opacity = node.lamp.opacity ?? 0
      const lamp = new THREE.MeshBasicMaterial({ color: node.lamp.off, transparent: opacity > 0, opacity: opacity || 1 })
      this.lamps.push({ material: lamp, state: node.lamp.state, on: new THREE.Color(node.lamp.on).getHex(), off: new THREE.Color(node.lamp.off).getHex() })
      material = lamp
    }
    const mesh = new THREE.Mesh(geometry, material)
    mesh.applyMatrix4(matrix)
    mesh.userData.press = node.press
    mesh.userData.toggle = node.toggle
    mesh.userData.noPick = node.noPick
    object.add(mesh)
  }

  private pick(event: { clientX: number; clientY: number }) {
    const rect = this.canvas.getBoundingClientRect()
    const pointer = new THREE.Vector2(((event.clientX - rect.left) / rect.width) * 2 - 1, -((event.clientY - rect.top) / rect.height) * 2 + 1)
    this.raycaster.setFromCamera(pointer, this.camera)
    for (const hit of this.raycaster.intersectObject(this.root, true)) {
      if (hit.object.userData.noPick) continue
      let instance: string | null = null
      let press: string | null = null
      let toggle = false
      for (let o: THREE.Object3D | null = hit.object; o; o = o.parent) {
        if (!press && o.userData.press) {
          press = o.userData.press
          toggle = !!o.userData.toggle
        }
        if (o.userData.instance) {
          instance = o.userData.instance
          break
        }
      }
      if (instance) return { instance, press, toggle }
    }
    return { instance: null, press: null, toggle: false }
  }

  // Marks what a station check is about: a red box around each module and a dot at the spot.
  highlight(instances: string[], at: { x: number; y: number; z: number } | null) {
    for (const child of [...this.marks.children]) {
      this.marks.remove(child)
      if (child instanceof THREE.Box3Helper) child.dispose()
    }
    for (const id of instances) {
      const box = this.moduleBox(id)
      if (box) this.marks.add(new THREE.Box3Helper(box, 0xe5484d))
    }
    if (at) {
      const dot = new THREE.Mesh(new THREE.SphereGeometry(6, 16, 12), new THREE.MeshBasicMaterial({ color: 0xe5484d, depthTest: false }))
      dot.renderOrder = 11
      dot.position.copy(this.root.localToWorld(new THREE.Vector3(at.x, at.y, at.z)))
      this.marks.add(dot)
    }
    this.invalidate()
  }

  private readonly onPointerDown = (event: PointerEvent) => {
    if (event.button !== 0) return
    this.downAt = { x: event.clientX, y: event.clientY }
    if (this.build?.pointerDown(event)) return
    const { press, toggle } = this.pick(event)
    // In the builder the console's buttons are not pressed, only the module selected.
    if (press && !this.build) {
      this.pressed = { name: press, toggle }
      this.controls.enabled = false
      this.events.control(press, true, toggle)
    }
  }

  private readonly onPointerUp = (event: PointerEvent) => {
    if (this.pressed) {
      this.events.control(this.pressed.name, false, this.pressed.toggle)
      this.pressed = null
      this.controls.enabled = true
    }
    if (!this.downAt || event.target !== this.canvas) {
      this.downAt = null
      return
    }
    const moved = Math.hypot(event.clientX - this.downAt.x, event.clientY - this.downAt.y)
    this.downAt = null
    if (this.build?.pointerUp(event, moved < DRAG_THRESHOLD)) return
    if (moved >= DRAG_THRESHOLD) return
    this.events.select(this.pick(event).instance)
  }

  private readonly onPointerMove = (event: PointerEvent) => {
    if (this.build?.pointerMove(event)) return
    if (this.build) return
    if (event.buttons) return
    this.canvas.style.cursor = this.pick(event).press ? 'pointer' : ''
  }

  private readonly onDoubleClick = (event: MouseEvent) => {
    const { instance, press } = this.pick(event)
    if (press) return
    const box = instance ? this.moduleBox(instance) : null
    if (!box) {
      this.home()
      this.invalidate()
      return
    }
    const center = box.getCenter(new THREE.Vector3())
    const distance = Math.max(box.getSize(new THREE.Vector3()).length() * 1.7, this.controls.minDistance)
    const direction = this.camera.position.clone().sub(this.controls.target).normalize()
    this.controls.target.copy(center)
    this.camera.position.copy(center).addScaledVector(direction, distance)
    this.controls.update()
    this.invalidate()
  }

  private home(keep = false) {
    // A station much larger than a stand, such as a line at real scale, is framed as a whole.
    if (this.options.fit || this.large()) {
      this.fit()
      return
    }
    // The same station again, as in the builder after each change, keeps where the camera was.
    const view = keep ? this.options.view : undefined
    this.camera.position.copy(view ? new THREE.Vector3().fromArray(view.position) : HOME_POSITION)
    this.controls.target.copy(view ? new THREE.Vector3().fromArray(view.target) : HOME_TARGET)
    this.controls.update()
  }

  // large tells whether the station is too large for the home view, which frames a stand.
  private large() {
    this.scene.updateMatrixWorld(true)
    return new THREE.Box3().setFromObject(this.root).getSize(new THREE.Vector3()).length() > LARGE_STATION
  }

  // Frames everything there is, for things of any size from a connector to a station.
  private fit() {
    this.scene.updateMatrixWorld(true)
    const box = new THREE.Box3().setFromObject(this.root)
    const size = Math.max(box.getSize(new THREE.Vector3()).length(), 1)
    const distance = size / (2 * Math.tan(rad(CAMERA_FOV) / 2)) + size / 4
    this.camera.near = Math.max(0.5, distance / 200)
    this.camera.far = distance * 30
    this.camera.updateProjectionMatrix()
    this.controls.minDistance = size / 8
    this.controls.maxDistance = distance * 6
    const view = this.options.view
    if (view) {
      this.camera.position.fromArray(view.position)
      this.controls.target.fromArray(view.target)
    } else {
      const center = box.getCenter(new THREE.Vector3())
      this.controls.target.copy(center)
      this.camera.position.copy(center).addScaledVector(FIT_DIRECTION, distance)
    }
    this.controls.update()
  }

  view(): CameraView {
    return { position: this.camera.position.toArray(), target: this.controls.target.toArray() }
  }

  // A hidden view draws nothing, so the simulation costs no GPU behind another page.
  setActive(active: boolean) {
    this.active = active
    if (active) this.invalidate()
  }

  // Shows the selection as a bounding box, which costs one draw call.
  select(id: string | null) {
    if (this.selectionBox) {
      this.scene.remove(this.selectionBox)
      this.selectionBox.dispose()
      this.selectionBox = null
    }
    const box = id ? this.moduleBox(id) : null
    if (box) {
      this.selectionBox = new THREE.Box3Helper(box, SELECTION_COLOR)
      this.scene.add(this.selectionBox)
    }
    this.invalidate()
  }

  // A module's box includes the modules it contains.
  private moduleBox(id: string) {
    let box: THREE.Box3 | null = null
    for (const [name, group] of this.instances) {
      if (name === id || name.startsWith(`${id}/`)) {
        const part = new THREE.Box3().setFromObject(group)
        box = box ? box.union(part) : part
      }
    }
    return box
  }

  // newPiece is a workpiece of a kind as drawn: from its model, or its shape in its colour.
  private newPiece(kind: string): THREE.Object3D {
    const template = this.workpieceTemplates.get(kind)
    const piece = template ? template.clone() : new THREE.Mesh(this.workpieceGeometry.get(kind), this.workpieceMaterials.get(kind))
    piece.traverse((o) => (o.userData.noPick = true))
    piece.userData.kind = kind
    return piece
  }

  private syncWorkpieces(snapshot: Snapshot) {
    const seen = new Set<number>()
    const axis = new THREE.Vector3(0, 0, 1)
    const turn = new THREE.Quaternion()
    for (const piece of snapshot.workpieces ?? []) {
      seen.add(piece.id)
      let mesh = this.workpieceMeshes.get(piece.id)
      if (!mesh) {
        mesh = this.newPiece(piece.kind)
        this.workpieceMeshes.set(piece.id, mesh)
        this.root.add(mesh)
      }
      mesh.position.copy(v(piece.at))
      // Upright as its transport tilts it, and turned to its heading.
      mesh.quaternion.setFromUnitVectors(axis, v(piece.up))
      const heading = piece.heading ?? { x: 1, y: 0, z: 0 }
      mesh.quaternion.multiply(turn.setFromAxisAngle(axis, Math.atan2(heading.y, heading.x)))
      const look = JSON.stringify([piece.loads ?? [], piece.marks ?? {}])
      if (mesh.userData.look !== look) {
        mesh.userData.look = look
        this.dressWorkpiece(mesh, piece)
      }
    }
    for (const [id, mesh] of this.workpieceMeshes) {
      if (!seen.has(id)) {
        this.root.remove(mesh)
        this.workpieceMeshes.delete(id)
      }
    }
  }

  // What was set onto a workpiece, where it was put, such as a cap or boxes on a pallet, and
  // what processes left on it: a filling that rises inside it, and holes into its top.
  private dressWorkpiece(mesh: THREE.Object3D, piece: WorkpieceView) {
    for (const child of [...mesh.children]) {
      if (!child.userData.dressing) continue
      mesh.remove(child)
      if (child instanceof THREE.Mesh && child.userData.own) child.geometry.dispose()
    }
    const heightOf = (kind: string) => this.workpieceKinds.get(kind)?.height || this.workpieceHeight
    const own = heightOf(piece.kind)
    for (const load of piece.loads ?? []) {
      const joined = this.newPiece(load.kind)
      joined.position.set(load.at.x, load.at.y, load.at.z)
      joined.rotation.z = Math.atan2(load.heading?.y ?? 0, load.heading?.x ?? 1)
      joined.userData.dressing = true
      mesh.add(joined)
    }
    for (const [id, amount] of Object.entries(piece.marks ?? {})) {
      const mark = this.markDefs.get(id)
      if (!mark || !amount) continue
      if (mark.fill) {
        const height = Math.min(amount * mark.fill, own - this.workpieceFloor - 0.5)
        const geometry = new THREE.CylinderGeometry(this.workpieceInner, this.workpieceInner, height, 32).rotateX(Math.PI / 2)
        const fill = new THREE.Mesh(geometry, this.markMaterials.get(id) ?? this.holeMaterial)
        fill.position.z = this.workpieceFloor + height / 2
        fill.userData = { noPick: true, dressing: true, own: true }
        mesh.add(fill)
      }
      if (mark.hole && !(piece.loads ?? []).length) {
        const hole = new THREE.Mesh(new THREE.CircleGeometry(mark.hole / 2, 24), this.holeMaterial)
        // Into the floor of a cup-shaped workpiece, or its top.
        hole.position.z = (this.workpieceFloor || own) + 0.05
        hole.userData = { noPick: true, dressing: true, own: true }
        mesh.add(hole)
      }
    }
  }

  // Renders only when something visible changed, so an idle station costs no GPU.
  update(snapshot: Snapshot) {
    const states = snapshot.states ?? []
    const signature = JSON.stringify([states, snapshot.workpieces])
    if (signature === this.signature) return
    this.signature = signature

    this.syncWorkpieces(snapshot)
    for (const belt of this.belts) belt.texture.offset.x = -states[belt.state] / 40
    const step = new THREE.Matrix4()
    for (const { object, base, node } of this.moving) {
      const joint = node.joint!
      const value = joint.from + (joint.to - joint.from) * states[joint.state]
      const o = joint.origin
      const axis = v(joint.axis)
      object.matrix.copy(base)
      if (joint.type === 'revolute') {
        object.matrix
          .multiply(step.makeTranslation(o.x, o.y, o.z))
          .multiply(step.makeRotationAxis(axis.normalize(), rad(value)))
          .multiply(step.makeTranslation(-o.x, -o.y, -o.z))
      } else {
        object.matrix.multiply(step.makeTranslation(axis.x * value, axis.y * value, axis.z * value))
      }
      object.matrixWorldNeedsUpdate = true
    }
    for (const lamp of this.lamps) lamp.material.color.setHex(lamp.state < 0 || states[lamp.state] >= 0.5 ? lamp.on : lamp.off)
    for (const range of this.lampRanges) {
      const lit = range.state < 0 || states[range.state] >= 0.5
      if (lit === range.shown) continue
      range.shown = lit
      const color = lit ? range.on : range.off
      for (let i = range.start; i < range.start + range.count; i++) {
        range.colors.setXYZ(i, color.r, color.g, color.b)
        if (range.colors.itemSize === 4) range.colors.setW(i, lit ? 1 : 0)
      }
      range.colors.needsUpdate = true
    }
    this.invalidate()
  }

  resize(width: number, height: number) {
    this.renderer.setSize(width, height, false)
    this.camera.aspect = width / height
    this.camera.updateProjectionMatrix()
    this.invalidate()
  }

  private invalidate() {
    if (this.frame || !this.active) return
    this.frame = requestAnimationFrame(() => {
      this.frame = 0
      this.build?.beforeRender()
      this.renderer.render(this.scene, this.camera)
      this.events.rendered()
    })
  }

  // Renderer load, shown in the viewer: draw calls and triangles.
  stats() {
    return { calls: this.renderer.info.render.calls, triangles: this.renderer.info.render.triangles }
  }

  // Where to anchor a popup: the top of a module, in pixels within the canvas.
  anchor(id: string) {
    const group = this.instances.get(id)
    if (!group) return null
    const box = new THREE.Box3().setFromObject(group)
    const top = box.getCenter(new THREE.Vector3())
    top.y = box.max.y
    const point = top.project(this.camera)
    const rect = this.canvas.getBoundingClientRect()
    return { x: ((point.x + 1) / 2) * rect.width, y: ((1 - point.y) / 2) * rect.height }
  }

  // Screen positions for driving the 3D view in UI tests.
  controlPosition(name: string) {
    let found: THREE.Object3D | null = null
    this.root.traverse((o) => {
      if (o.userData.press === name) found = o
    })
    return found ? this.screenPosition(found) : null
  }

  stationPoint(x: number, y: number, z: number) {
    const point = this.root.localToWorld(new THREE.Vector3(x, y, z)).project(this.camera)
    const rect = this.canvas.getBoundingClientRect()
    return { x: rect.left + ((point.x + 1) / 2) * rect.width, y: rect.top + ((1 - point.y) / 2) * rect.height }
  }

  private screenPosition(object: THREE.Object3D) {
    const center = new THREE.Box3().setFromObject(object).getCenter(new THREE.Vector3()).project(this.camera)
    const rect = this.canvas.getBoundingClientRect()
    return { x: rect.left + ((center.x + 1) / 2) * rect.width, y: rect.top + ((1 - center.y) / 2) * rect.height }
  }

  dispose() {
    cancelAnimationFrame(this.frame)
    this.build?.dispose()
    this.canvas.removeEventListener('pointerdown', this.onPointerDown, { capture: true })
    this.canvas.removeEventListener('pointermove', this.onPointerMove)
    this.canvas.removeEventListener('dblclick', this.onDoubleClick)
    window.removeEventListener('pointerup', this.onPointerUp)
    window.removeEventListener('pointercancel', this.onPointerUp)
    this.controls.dispose()
    this.renderer.dispose()
    // Each page can open many previews, and browsers limit the WebGL contexts.
    this.renderer.forceContextLoss()
  }
}
