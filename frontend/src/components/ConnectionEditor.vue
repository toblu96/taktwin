<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { choose } from '../ask'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { Preview, SignalInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import type { Node } from '../../bindings/github.com/toblu96/taktwin/internal/link/models'
import { savedConnection, simulation } from '../sim'
import UiCheckbox from '../ui/UiCheckbox.vue'
import UiSegmented from '../ui/UiSegmented.vue'
import UiSelect from '../ui/UiSelect.vue'
import UiTree, { type TreeRow } from '../ui/UiTree.vue'
import IconIn from '~icons/fluent/arrow-right-16-regular'
import IconOut from '~icons/fluent/arrow-left-16-regular'
import IconRefresh from '~icons/fluent/arrow-clockwise-16-regular'
import IconAdd from '~icons/fluent/add-16-regular'
import IconClose from '~icons/fluent/dismiss-16-regular'
import IconDelete from '~icons/fluent/delete-16-regular'
import IconUnbind from '~icons/fluent/link-dismiss-16-regular'

// The editor of a connection, in the middle of the wiring: which item of the controller each
// signal of the station is exchanged with, picked from what the controller offers, browsed in
// the side bar (#wiring-nodes, which the wiring view puts there while this is open), or written by hand, and the templates that bind many at once; or the whole file as
// JSON. It saves the connection into the user's folder.
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

// The bindings are kept when they are saved, not with each change as the station is: what has
// changed since shows, and closing or leaving the wiring asks first.
const savedText = ref('')
const unsaved = computed(() => !!doc.value && text.value !== savedText.value)
async function keepOrDrop(): Promise<boolean> {
  if (!unsaved.value) return true
  const answer = await choose('The bindings have changes that are not saved yet.', ['Save', 'Discard'])
  if (answer === 'Save') return save(false)
  return answer === 'Discard'
}
async function close() {
  if (await keepOrDrop()) emit('close')
}
onBeforeRouteLeave(() => keepOrDrop())
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
    savedText.value = text.value
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
// A signal that nothing of the station reaches, with no symbol and nothing it is for, need not
// be exchanged: it can be bound, but it is not missing when it is not.
const used = (s: SignalInfo) => !!(s.symbol || s.description)
const missing = (s: SignalInfo) => used(s) && unbound.value.has(s.address)

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

// The nodes as rows of a tree, as far as they are open: one with an item binds the selected
// signal, one with nodes below opens.
const byKey = new Map<string, Node>()
const tree = computed<TreeRow[]>(() => {
  byKey.clear()
  const out: TreeRow[] = []
  const walk = (under: string, depth: number, parent?: string) => {
    for (const n of nodes[under] ?? []) {
      const key = `${depth}:${n.id}`
      byKey.set(key, n)
      out.push({
        key,
        label: n.name,
        depth,
        children: !!n.below,
        open: open.has(n.under ?? ''),
        parent,
        title: n.item ? `${n.id}: bind the selected signal to it` : n.id,
        hint: n.type ? `${n.type} ${n.readable ? 'R' : ''}${n.writable ? 'W' : ''}`.trim() : undefined,
      })
      if (n.below && open.has(n.under ?? '')) walk(n.under ?? '', depth + 1, key)
    }
  }
  walk('', 0)
  return out
})
function pickRow(key: string) {
  const n = byKey.get(key)
  if (!n) return
  if (n.item) pick(n)
  else if (n.below) void toggle(n)
}
function openRow(key: string, to: boolean) {
  const n = byKey.get(key)
  if (n && open.has(n.under ?? '') !== to) void toggle(n)
}

const KINDS = [
  { value: 'inputs', label: 'Input bits' },
  { value: 'outputs', label: 'Output bits' },
  { value: 'analogInputs', label: 'Analogue inputs' },
  { value: 'analogOutputs', label: 'Analogue outputs' },
  { value: 'all', label: 'All signals' },
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

// The JSON is taken back when going back to the bindings, unless it does not read.
const view = computed({
  get: () => tab.value,
  set: (to: 'bindings' | 'json') => {
    if (to === 'json') {
      json.value = text.value
      jsonError.value = ''
      tab.value = 'json'
      return
    }
    try {
      doc.value = JSON.parse(json.value)
      jsonError.value = ''
      tab.value = 'bindings'
    } catch (e) {
      jsonError.value = (e as Error).message
    }
  },
})

const onlyHere = computed({
  get: () => !!doc.value?.station,
  set: (on: boolean) => {
    if (doc.value) doc.value.station = on ? station.value : undefined
  },
})

// The file's id from the connection's name, kept apart from the ids of the app's own connections.
function idOf(name: string) {
  const id =
    name
      .toLowerCase()
      .normalize('NFKD')
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '') || 'my-connection'
  return /^(codesys|soft|opcua)(-|$)/.test(id) ? `my-${id}` : id
}

// Saves the bindings, and closes the editor unless asked to stay; it says whether it saved.
async function save(close = true): Promise<boolean> {
  if (!doc.value) return false
  if (!doc.value.id?.trim()) doc.value.id = idOf(doc.value.name ?? '')
  busy.value = true
  message.value = ''
  try {
    const info = await Service.SaveConnection(props.plc, text.value, props.values)
    savedText.value = text.value
    savedConnection.value = { plc: props.plc, id: info.id, values: props.values }
    if (close) emit('close')
    return true
  } catch (e) {
    message.value = (e as Error).message
    return false
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
  const inUse = signals.value.filter(used)
  const bound = inUse.filter((s) => !unbound.value.has(s.address)).length
  return `${bound} of the ${inUse.length} signals in use exchanged`
})
</script>

<template>
  <section class="flex min-h-0 flex-col bg-s0" aria-label="Bindings of the connection">
    <header class="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-0 border-b border-solid border-line bg-s1 px-3 py-2">
      <h2 class="m-0 text-[13px] font-semibold">Bindings</h2>
      <template v-if="doc">
        <label class="flex items-center gap-1.5 text-xs text-muted">Name <input v-model="doc.name" class="t-input h-6 w-48 px-1.5 text-[13px]" type="text" /></label>
        <label class="flex items-center gap-1.5 text-xs text-muted"
          >Id <input v-model="doc.id" class="t-input h-6 w-32 px-1.5 font-mono text-xs" type="text" placeholder="my-plc" spellcheck="false"
        /></label>
        <UiCheckbox v-model="onlyHere" label="Only for this station" class="text-xs" />
      </template>
      <span class="flex-1" />
      <UiSegmented
        v-model="view"
        label="Show"
        :options="[
          { value: 'bindings', label: 'Bindings' },
          { value: 'json', label: 'JSON' },
        ]"
      />
      <button v-if="yours" class="t-btn text-danger" @click="remove"><IconDelete class="size-4" />Delete</button>
      <span v-if="unsaved" class="text-xs text-accent-ink" role="status">Not saved yet</span>
      <button
        class="t-btn t-btn-primary"
        :disabled="busy || !doc"
        :title="yours ? undefined : 'Saves it into your folder, with an id from its name unless it has one'"
        @click="save()"
      >
        {{ yours ? 'Save' : 'Save into your folder' }}
      </button>
      <button class="t-icon-btn" title="Close the bindings" aria-label="Close the bindings" @click="close"><IconClose class="size-4" /></button>
    </header>
    <p v-if="message" class="m-0 shrink-0 border-0 border-b border-solid border-line px-3 py-2 text-danger" role="alert">{{ message }}</p>
    <p v-else-if="preview?.error" class="m-0 shrink-0 border-0 border-b border-solid border-line px-3 py-2 text-danger">{{ preview.error }}</p>

    <div v-if="tab === 'json'" class="flex min-h-0 flex-1 flex-col gap-2 p-3">
      <p class="m-0 text-xs text-muted">The whole connection, as its file has it: vars as a JSON Schema, the driver's settings, namespaces, bindings and templates.</p>
      <textarea
        v-model="json"
        class="t-input h-auto min-h-0 flex-1 resize-none py-2 font-mono text-xs leading-relaxed select-text"
        spellcheck="false"
        aria-label="The connection as JSON"
      />
      <p v-if="jsonError" class="m-0 text-danger" role="alert">{{ jsonError }}</p>
    </div>

    <div v-else-if="doc" class="flex min-h-0 flex-1">
      <div class="min-w-0 flex-1 overflow-auto">
        <section class="t-section">
          <p class="m-0 mb-3 text-xs text-muted">
            Select a signal, then pick its node on the controller in the side bar, or type its NodeId. A binding follows the signal's symbol when it moves to another pin, or stays
            with the address.
            <span class="font-medium text-fg">{{ counts }}.</span>
          </p>
          <table class="w-full table-fixed border-collapse">
            <colgroup>
              <col class="w-7" />
              <col class="w-[36%]" />
              <col />
              <col class="w-24" />
              <col class="w-10" />
              <col class="w-9" />
            </colgroup>
            <thead>
              <tr class="text-left text-[11px] font-semibold tracking-wide text-muted uppercase">
                <th class="pb-1.5 font-semibold" />
                <th class="pb-1.5 font-semibold">Signal</th>
                <th class="pb-1.5 font-semibold">Node</th>
                <th class="pb-1.5 font-semibold">Follows</th>
                <th class="pb-1.5 font-semibold" title="Exchanged the other way round">Not</th>
                <th class="pb-1.5 font-semibold"><span class="sr-only">Clear</span></th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="s in signals"
                :key="s.address"
                class="group border-0 border-t border-solid border-line"
                :class="s.address === selected ? 'bg-highlight' : ''"
                :data-selected="s.address === selected || undefined"
                :title="s.description"
                @click="selected = s.address"
              >
                <td class="py-1 pl-1">
                  <component
                    :is="s.input ? IconIn : IconOut"
                    class="size-4"
                    :class="s.input ? 'text-lamp-in' : 'text-lamp-out'"
                    :aria-label="s.input ? 'Taktwin writes it' : 'Taktwin reads it'"
                  />
                </td>
                <td class="py-1 pr-3 leading-tight">
                  <span class="flex items-baseline gap-2">
                    <span v-if="s.symbol" class="font-mono text-xs font-semibold">{{ s.symbol }}</span>
                    <span class="font-mono text-[11px] text-muted group-data-selected:text-fg/80" :title="s.address">{{ s.shown }}<template v-if="s.analog"> ~</template></span>
                  </span>
                  <span class="block truncate text-xs text-muted group-data-selected:text-fg/80">{{ s.description || 'Not used' }}</span>
                </td>
                <td class="py-1 pr-2">
                  <input
                    class="t-input h-6 px-1.5 font-mono text-xs select-text"
                    :class="missing(s) ? 'border-danger' : ''"
                    type="text"
                    :value="itemText(bindingOf(s)?.item)"
                    :placeholder="generatedText(s) || '—'"
                    :aria-label="`Node of ${s.shown}`"
                    spellcheck="false"
                    @focus="selected = s.address"
                    @change="typed(s, $event)"
                  />
                </td>
                <td class="py-1 pr-2">
                  <UiSelect
                    v-if="bindingOf(s) && s.symbol"
                    hide-label
                    :label="`What the binding of ${s.shown} follows`"
                    :options="[
                      { value: 'symbol', label: 'Symbol' },
                      { value: 'address', label: 'Address' },
                    ]"
                    :model-value="bindingOf(s)?.address ? 'address' : 'symbol'"
                    @update:model-value="(v: string | null) => v && setKey(s, v)"
                  />
                </td>
                <td class="py-1">
                  <UiCheckbox
                    v-if="bindingOf(s) && !s.analog"
                    hide-label
                    :label="`Exchange ${s.shown} the other way round`"
                    :model-value="!!bindingOf(s)?.invert"
                    @update:model-value="(on: boolean) => setInvert(s, on)"
                  />
                </td>
                <td class="py-1 text-right">
                  <button
                    v-if="bindingOf(s)"
                    class="t-icon-btn size-6"
                    :title="`Clear the node of ${s.symbol || s.shown}`"
                    :aria-label="`Clear the node of ${s.symbol || s.shown}`"
                    @click.stop="setItem(s, null)"
                  >
                    <IconUnbind class="size-4" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </section>
        <section class="t-section">
          <h3 class="t-section-title">Templates</h3>
          <p class="m-0 mb-3 text-xs text-muted">
            Bind every signal of a kind at once. A template's NodeId may use {shown} (as the PLC shows the address), {address}, {symbol}, {byte}, {bit}, {bit:2} and {channel}, and
            the connection's vars. Bindings above take the place of what a template gives them.
          </p>
          <div v-for="(g, i) in doc.generate ?? []" :key="i" class="mb-2 flex items-center gap-2">
            <UiSelect class="w-44 shrink-0" hide-label label="Signals" :options="KINDS" :model-value="g.signals" @update:model-value="(v: string | null) => v && (g.signals = v)" />
            <input
              class="t-input font-mono text-xs select-text"
              type="text"
              :value="itemText(g.item)"
              spellcheck="false"
              aria-label="NodeId of the template"
              @change="setTemplate(g, $event)"
            />
            <button class="t-btn shrink-0" @click="doc.generate?.splice(i, 1)">Remove</button>
          </div>
          <button class="t-btn" @click="addTemplate"><IconAdd class="size-4" />Add a template</button>
        </section>
      </div>

      <!-- What the controller offers takes the side bar's place, beside the signals. -->
      <Teleport defer to="#wiring-nodes">
        <aside class="flex h-full min-w-0 flex-col bg-s1" aria-label="On the controller">
          <div class="flex h-9 shrink-0 items-center gap-2 px-3">
            <h3 class="m-0 flex-1 text-xs font-semibold tracking-wide text-muted uppercase">On the controller</h3>
            <button class="t-icon-btn size-6" :disabled="!!browsing" title="Look again" aria-label="Look again on the controller" @click="browse('')">
              <IconRefresh class="size-4" />
            </button>
          </div>
          <p v-if="browsing" class="m-0 px-3 pb-2 text-xs text-muted">Looking…</p>
          <p v-else-if="!tree.length" class="m-0 px-3 pb-2 text-xs text-muted">Nothing found: is the controller running, and does the connection's form say where it is?</p>
          <UiTree :items="tree" label="Nodes on the controller" class="min-h-0 flex-1 overflow-auto px-1.5 pb-2" @select="pickRow" @toggle="openRow" />
        </aside>
      </Teleport>
    </div>
  </section>
</template>
