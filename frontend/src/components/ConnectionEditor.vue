<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { Preview, SignalInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import type { Node } from '../../bindings/github.com/toblu96/taktwin/internal/link/models'
import { savedConnection, simulation } from '../sim'

// The editor of a connection: which item of the controller each signal of the station is
// exchanged with, picked from what the controller offers or written by hand, and the
// templates that bind many at once. It saves the connection into the user's folder.
const props = defineProps<{ plc: string; id: string; values: Record<string, unknown> }>()
const emit = defineEmits<{ close: [] }>()

type BindingDef = { signal?: string; address?: string; item: Record<string, unknown>; invert?: boolean; scale?: unknown }
type GenerateDef = { signals: string; item: Record<string, unknown> }
type Doc = {
  id: string
  name: string
  description?: string
  station?: string
  plc?: string
  driver: string
  vars?: unknown
  settings?: unknown
  namespaces?: Record<string, string>
  bindings?: BindingDef[]
  generate?: GenerateDef[]
}

const doc = ref<Doc | null>(null)
const yours = ref(false)
const builtIn = ref(false)
const signals = ref<SignalInfo[]>([])
const preview = ref<Preview | null>(null)
const message = ref('')
const busy = ref(false)
const tab = ref<'bindings' | 'json'>('bindings')
const json = ref('')
const jsonError = ref('')
const selected = ref('')

const text = computed(() => (doc.value ? JSON.stringify(doc.value, null, 2) : ''))
const station = computed(() => simulation.scene.value?.id ?? '')

onMounted(async () => {
  try {
    const [d, list] = await Promise.all([Service.ConnectionFor(props.plc, props.id), Service.SignalsFor(props.plc)])
    signals.value = list ?? []
    const parsed: Doc = JSON.parse(d.json)
    yours.value = d.yours
    // A connection of the app is the start of one's own, which needs an id of its own.
    builtIn.value = d.info.source === 'Taktwin'
    if (builtIn.value) {
      parsed.id = ''
      parsed.name = `${parsed.name} (mine)`
      parsed.description = undefined
      parsed.bindings ??= []
    }
    doc.value = parsed
    selected.value = signals.value[0]?.address ?? ''
    await browse('')
  } catch (e) {
    message.value = (e as Error).message
  }
})

// What the connection exchanges, as it is now.
let previewTimer = 0
watch(text, () => {
  window.clearTimeout(previewTimer)
  previewTimer = window.setTimeout(async () => {
    preview.value = await Service.PreviewConnection(props.plc, text.value, props.values)
  }, 200)
})

const resolved = computed(() => new Map((preview.value?.bindings ?? []).map((b) => [b.address, b])))
const unbound = computed(() => new Set(preview.value?.unbound ?? []))

// The binding a row edits: the first that names its signal, by symbol or by address.
function bindingOf(s: SignalInfo): BindingDef | undefined {
  return doc.value?.bindings?.find((b) => (b.signal && b.signal === s.symbol) || b.address === s.address)
}

function itemText(item: Record<string, unknown> | undefined): string {
  if (!item) return ''
  if (typeof item.nodeId === 'string') return item.nodeId
  if (typeof item.path === 'string') return item.path
  return JSON.stringify(item)
}

function generatedText(s: SignalInfo): string {
  const b = resolved.value.get(s.address)
  if (!b?.generated) return ''
  try {
    return itemText(JSON.parse(b.item))
  } catch {
    return b.item
  }
}

// Sets the item of a row's binding, by symbol where the signal has one; an empty one removes it.
function setItem(s: SignalInfo, item: Record<string, unknown> | null) {
  const d = doc.value
  if (!d) return
  d.bindings ??= []
  const b = bindingOf(s)
  if (!item) {
    if (b) d.bindings.splice(d.bindings.indexOf(b), 1)
    return
  }
  if (b) b.item = item
  else d.bindings.push(s.symbol ? { signal: s.symbol, item } : { address: s.address, item })
}

function typed(s: SignalInfo, event: Event) {
  const v = (event.target as HTMLInputElement).value.trim()
  setItem(s, v ? (v.startsWith('/') ? { path: v } : { nodeId: v }) : null)
}

function setKey(s: SignalInfo, key: string) {
  const b = bindingOf(s)
  if (!b) return
  if (key === 'address') {
    delete b.signal
    b.address = s.address
  } else {
    delete b.address
    b.signal = s.symbol
  }
}

function setInvert(s: SignalInfo, on: boolean) {
  const b = bindingOf(s)
  if (b) b.invert = on || undefined
}

// The controller's nodes, by the node they are below ("" for the top).
const nodes = reactive<Record<string, Node[]>>({})
const open = reactive(new Set<string>())
const browsing = ref('')

async function browse(under: string) {
  browsing.value = under || 'top'
  try {
    nodes[under] = (await Service.BrowseConnection(text.value, props.values, under)) ?? []
    message.value = ''
  } catch (e) {
    message.value = (e as Error).message
  } finally {
    browsing.value = ''
  }
}

async function toggle(n: Node) {
  if (open.has(n.under ?? '')) {
    open.delete(n.under ?? '')
    return
  }
  open.add(n.under ?? '')
  if (!nodes[n.under ?? '']) await browse(n.under ?? '')
}

// Picking a node binds the selected signal to it, and moves on to the next.
function pick(n: Node) {
  const s = signals.value.find((x) => x.address === selected.value)
  if (!s || !n.item) return
  setItem(s, n.item as unknown as Record<string, unknown>)
  const i = signals.value.indexOf(s)
  if (i + 1 < signals.value.length) selected.value = signals.value[i + 1].address
}

// The nodes as a list, with their depth, as far as they are open.
const tree = computed(() => {
  const out: { node: Node; depth: number }[] = []
  const walk = (under: string, depth: number) => {
    for (const n of nodes[under] ?? []) {
      out.push({ node: n, depth })
      if (n.below && open.has(n.under ?? '')) walk(n.under ?? '', depth + 1)
    }
  }
  walk('', 0)
  return out
})

const KINDS = [
  { id: 'inputs', label: 'Input bits' },
  { id: 'outputs', label: 'Output bits' },
  { id: 'analogInputs', label: 'Analogue inputs' },
  { id: 'analogOutputs', label: 'Analogue outputs' },
  { id: 'all', label: 'All signals' },
]

function addTemplate() {
  const d = doc.value
  if (!d) return
  d.generate ??= []
  d.generate.push({ signals: 'inputs', item: { nodeId: 'ns=2;s={shown}' } })
}

function setTemplate(g: GenerateDef, event: Event) {
  g.item = { nodeId: (event.target as HTMLInputElement).value }
}

function showJson() {
  json.value = text.value
  jsonError.value = ''
  tab.value = 'json'
}

function applyJson() {
  try {
    doc.value = JSON.parse(json.value)
    jsonError.value = ''
    tab.value = 'bindings'
  } catch (e) {
    jsonError.value = (e as Error).message
  }
}

const onlyHere = computed({
  get: () => !!doc.value?.station,
  set: (on: boolean) => {
    if (doc.value) doc.value.station = on ? station.value : undefined
  },
})

async function save() {
  if (!doc.value) return
  busy.value = true
  message.value = ''
  try {
    const info = await Service.SaveConnection(props.plc, text.value, props.values)
    savedConnection.value = { plc: props.plc, id: info.id, values: props.values }
    emit('close')
  } catch (e) {
    message.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function remove() {
  if (!doc.value) return
  try {
    await Service.DeleteConnection(doc.value.id)
    savedConnection.value = { plc: props.plc, id: '' }
    emit('close')
  } catch (e) {
    message.value = (e as Error).message
  }
}

const counts = computed(() => {
  const all = signals.value.length
  const bound = all - unbound.value.size
  return `${bound} of ${all} signals exchanged`
})
</script>

<template>
  <div class="editor" role="dialog" aria-label="Connection editor">
    <div class="toolbar">
      <strong class="title">Connection</strong>
      <template v-if="doc">
        <label>Name <input v-model="doc.name" class="name" type="text" /></label>
        <label>Id <input v-model="doc.id" class="mono id" type="text" placeholder="my-plc" spellcheck="false" /></label>
        <label class="check"><input v-model="onlyHere" type="checkbox" /> Only for this station</label>
      </template>
      <span class="spacer" />
      <button v-if="tab === 'bindings'" @click="showJson">JSON</button>
      <button v-else @click="applyJson">Back to the bindings</button>
      <button v-if="yours" class="danger" @click="remove">Delete</button>
      <button class="primary" :disabled="busy || !doc || !doc.id" :title="yours ? '' : 'Saves it into your folder'" @click="save">
        {{ yours ? 'Save' : 'Save into your folder' }}
      </button>
      <button @click="emit('close')">Close</button>
    </div>
    <p v-if="message" class="error" role="alert">{{ message }}</p>
    <p v-else-if="preview?.error" class="error">{{ preview.error }}</p>
    <div v-if="tab === 'json'" class="json">
      <p class="help">The whole connection, as its file has it: vars as a JSON Schema, the driver's settings, namespaces, bindings and templates.</p>
      <textarea v-model="json" class="mono" spellcheck="false" />
      <p v-if="jsonError" class="error">{{ jsonError }}</p>
    </div>
    <div v-else-if="doc" class="body">
      <section class="signals">
        <p class="help">
          Select a signal, then pick the node on the right, or type its NodeId. A binding follows the signal's symbol when it moves to another pin, or stays with the address.
          {{ counts }}.
        </p>
        <table>
          <thead>
            <tr>
              <th />
              <th>Address</th>
              <th>Symbol</th>
              <th>Item</th>
              <th>By</th>
              <th title="Exchanged the other way round">Not</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="s in signals"
              :key="s.address"
              :class="{ selected: s.address === selected, unbound: unbound.has(s.address) }"
              :title="s.description"
              @click="selected = s.address"
            >
              <td class="dir" :title="s.input ? 'Taktwin writes it' : 'Taktwin reads it'">{{ s.input ? '→' : '←' }}<span v-if="s.analog">~</span></td>
              <td class="mono" :title="s.address">{{ s.shown }}</td>
              <td class="mono">{{ s.symbol }}</td>
              <td>
                <input
                  class="mono item"
                  type="text"
                  :value="itemText(bindingOf(s)?.item)"
                  :placeholder="generatedText(s) || '—'"
                  spellcheck="false"
                  @focus="selected = s.address"
                  @change="typed(s, $event)"
                />
              </td>
              <td>
                <select
                  v-if="bindingOf(s)"
                  :value="bindingOf(s)?.address ? 'address' : 'symbol'"
                  :disabled="!s.symbol"
                  @change="setKey(s, ($event.target as HTMLSelectElement).value)"
                >
                  <option value="symbol">symbol</option>
                  <option value="address">address</option>
                </select>
              </td>
              <td>
                <input v-if="bindingOf(s) && !s.analog" type="checkbox" :checked="!!bindingOf(s)?.invert" @change="setInvert(s, ($event.target as HTMLInputElement).checked)" />
              </td>
            </tr>
          </tbody>
        </table>
        <h4>Templates</h4>
        <p class="help">
          Bind every signal of a kind at once. A template's NodeId may use {shown} (as the PLC shows the address), {address}, {symbol}, {byte}, {bit}, {bit:2} and {channel}, and
          the connection's vars. Bindings above take the place of what a template gives them.
        </p>
        <div v-for="(g, i) in doc.generate ?? []" :key="i" class="template">
          <select v-model="g.signals">
            <option v-for="k in KINDS" :key="k.id" :value="k.id">{{ k.label }}</option>
          </select>
          <input class="mono" type="text" :value="itemText(g.item)" spellcheck="false" @change="setTemplate(g, $event)" />
          <button @click="doc.generate?.splice(i, 1)">Remove</button>
        </div>
        <button @click="addTemplate">Add a template</button>
      </section>
      <section class="nodes">
        <div class="nodes-head">
          <strong>On the controller</strong>
          <button :disabled="!!browsing" @click="browse('')">Refresh</button>
        </div>
        <p v-if="browsing" class="help">Looking…</p>
        <ul>
          <li v-for="{ node, depth } in tree" :key="node.id + depth" :style="{ paddingLeft: `${depth * 14}px` }">
            <button v-if="node.below" class="twist" :aria-label="open.has(node.under ?? '') ? 'Close' : 'Open'" @click="toggle(node)">
              {{ open.has(node.under ?? '') ? '▾' : '▸' }}
            </button>
            <span v-else class="twist" />
            <button class="node" :class="{ pickable: !!node.item }" :disabled="!node.item" :title="node.id" @click="pick(node)">
              {{ node.name }}
              <small v-if="node.type" class="mono">{{ node.type }} {{ node.readable ? 'R' : '' }}{{ node.writable ? 'W' : '' }}</small>
            </button>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>

<style scoped>
.editor {
  background: var(--bg);
  display: flex;
  flex-direction: column;
  inset: 0;
  min-height: 0;
  position: absolute;
  z-index: 5;
}

.toolbar {
  align-items: center;
  background: var(--panel);
  border-bottom: 1px solid var(--line);
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  padding: 6px 10px;
}

.toolbar label {
  align-items: center;
  color: var(--text-dim);
  display: flex;
  gap: 5px;
}

.toolbar input[type='text'] {
  background: var(--bg);
  border: 1px solid var(--line);
  color: var(--text);
  padding: 3px 6px;
}

.name {
  width: 220px;
}

.id {
  width: 140px;
}

.spacer {
  flex: 1;
}

.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--bg);
  font-weight: 600;
}

.danger {
  color: var(--danger);
}

.error {
  color: var(--danger);
  margin: 6px 10px;
}

.help {
  color: var(--text-dim);
  margin: 0 0 8px;
}

.body {
  display: grid;
  flex: 1;
  grid-template-columns: minmax(0, 1fr) 360px;
  min-height: 0;
}

.signals {
  overflow: auto;
  padding: 10px;
}

table {
  border-collapse: collapse;
  width: 100%;
}

th {
  color: var(--text-dim);
  font-weight: 400;
  text-align: left;
}

td,
th {
  border-bottom: 1px solid var(--line);
  padding: 2px 6px;
}

tr.selected td {
  background: color-mix(in srgb, var(--accent) 18%, transparent);
}

tr.unbound td:nth-child(2) {
  color: var(--text-dim);
}

.dir {
  color: var(--text-dim);
  width: 26px;
}

.item {
  background: var(--bg);
  border: 1px solid var(--line);
  color: var(--text);
  padding: 2px 5px;
  width: 100%;
}

.template {
  display: grid;
  gap: 6px;
  grid-template-columns: 150px minmax(0, 1fr) auto;
  margin-bottom: 6px;
}

.template input {
  background: var(--bg);
  border: 1px solid var(--line);
  color: var(--text);
  padding: 3px 6px;
}

h4 {
  margin: 14px 0 6px;
}

.nodes {
  border-left: 1px solid var(--line);
  overflow: auto;
  padding: 10px;
}

.nodes-head {
  align-items: center;
  display: flex;
  justify-content: space-between;
  margin-bottom: 6px;
}

.nodes ul {
  list-style: none;
  margin: 0;
  padding: 0;
}

.nodes li {
  align-items: center;
  display: flex;
  gap: 2px;
}

.twist {
  background: none;
  border: none;
  color: var(--text-dim);
  display: inline-block;
  padding: 0 2px;
  width: 16px;
}

.node {
  background: none;
  border: none;
  color: var(--text-dim);
  padding: 1px 4px;
  text-align: left;
}

.node.pickable {
  color: var(--text);
  cursor: pointer;
}

.node small {
  color: var(--text-dim);
  margin-left: 6px;
}

.json {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  padding: 10px;
}

.json textarea {
  background: var(--bg);
  border: 1px solid var(--line);
  color: var(--text);
  flex: 1;
  padding: 8px;
  resize: none;
}
</style>
