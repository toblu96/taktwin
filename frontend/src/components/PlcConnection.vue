<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { ConnectionInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { editingConnection, savedConnection, simulation } from '../sim'

const { plcId, plcView } = simulation

const MODES = [
  { id: 'none', label: 'None', help: 'No PLC: every output stays off. Switch them one by one with the I/O test, or connect a PLC.' },
  { id: 'soft', label: 'Soft PLC', help: "The app's own PLC, running the Structured Text program written in its editor, reached over OPC UA as any other PLC." },
  {
    id: 'external',
    label: 'External',
    help: 'The station connects to a controller outside the app, such as a CODESYS PLC or FluidSIM, through a connection: one of the app, or a connection file of your folder or a collection.',
  },
]

// A field of a connection's form, from a property of its vars (a JSON Schema).
type Field = {
  name: string
  type: string
  title: string
  description?: string
  required: boolean
  options?: { value: unknown; label: string }[]
  min?: number
  max?: number
  step?: number
  readOnly?: boolean
  secret?: boolean
  placeholder?: string
}

type Schema = {
  type?: string | string[]
  title?: string
  description?: string
  default?: unknown
  enum?: unknown[]
  oneOf?: { const?: unknown; title?: string }[]
  minimum?: number
  maximum?: number
  multipleOf?: number
  readOnly?: boolean
  writeOnly?: boolean
  examples?: unknown[]
  properties?: { [name: string]: Schema }
  required?: string[]
}

function fieldsOf(vars: string): Field[] {
  if (!vars) return []
  let schema: Schema
  try {
    schema = JSON.parse(vars)
  } catch {
    return []
  }
  return Object.entries(schema.properties ?? {}).map(([name, p]) => {
    const type = Array.isArray(p.type) ? p.type[0] : (p.type ?? 'string')
    const options = p.enum
      ? p.enum.map((v) => ({ value: v, label: String(v) }))
      : p.oneOf?.every((o) => 'const' in o)
        ? p.oneOf.map((o) => ({ value: o.const, label: o.title ?? String(o.const) }))
        : undefined
    return {
      name,
      type,
      title: p.title ?? name,
      description: p.description,
      required: schema.required?.includes(name) ?? false,
      options,
      min: p.minimum,
      max: p.maximum,
      step: p.multipleOf ?? (type === 'integer' ? 1 : undefined),
      readOnly: p.readOnly,
      secret: p.writeOnly,
      placeholder: p.examples?.length ? String(p.examples[0]) : undefined,
    }
  })
}

function defaultsOf(vars: string): { [name: string]: unknown } {
  try {
    const schema: Schema = JSON.parse(vars)
    return Object.fromEntries(Object.entries(schema.properties ?? {}).flatMap(([n, p]) => (p.default === undefined ? [] : [[n, p.default]])))
  } catch {
    return {}
  }
}

// What someone filled in for a connection, kept between sessions, without secrets.
function storedValues(id: string): { [name: string]: unknown } {
  try {
    const stored = JSON.parse(localStorage.getItem(`taktwin.connection.${id}`) ?? 'null')
    if (stored) return stored
    // The server address the app asked for before connections.
    const endpoint = localStorage.getItem('taktwin.endpoint')
    return id === 'codesys' && endpoint ? { endpoint } : {}
  } catch {
    return {}
  }
}

function storeValues(id: string, values: { [name: string]: unknown }, fields: Field[]) {
  const kept = Object.fromEntries(Object.entries(values).filter(([n]) => !fields.find((f) => f.name === n)?.secret))
  try {
    localStorage.setItem(`taktwin.connection.${id}`, JSON.stringify(kept))
  } catch {
    // It only means filling them in again next time.
  }
}

const mode = ref('none')
const connections = ref<ConnectionInfo[]>([])
const connection = ref('')
const values = ref<{ [name: string]: unknown }>({})
const message = ref('')
const busy = ref(false)

// What is running, to tell a change that still needs applying.
const applied = ref({ mode: 'none', connection: '', values: '{}' })

const chosen = computed(() => connections.value.find((c) => c.id === connection.value) ?? null)
const fields = computed(() => fieldsOf(chosen.value?.vars ?? ''))
const sources = computed(() => [...new Set(connections.value.map((c) => c.source))])

async function loadConnections() {
  try {
    connections.value = (await Service.ConnectionsFor(plcId.value)) ?? []
  } catch (error) {
    message.value = (error as Error).message
    connections.value = []
  }
  if (!connections.value.some((c) => c.id === connection.value)) connection.value = connections.value[0]?.id ?? ''
}

// Each connection keeps what was filled in for it.
watch(connection, (id) => {
  const c = connections.value.find((x) => x.id === id)
  values.value = { ...defaultsOf(c?.vars ?? ''), ...(id === applied.value.connection ? JSON.parse(applied.value.values) : storedValues(id)) }
})

onMounted(async () => {
  const plc = await Service.PLCFor(plcId.value)
  applied.value = { mode: plc.mode, connection: plc.connection, values: JSON.stringify(plc.values ?? {}) }
  mode.value = plc.mode
  await loadConnections()
  if (plc.connection) connection.value = plc.connection
})

const help = computed(() => MODES.find((m) => m.id === mode.value)?.help)

function editBindings() {
  editingConnection.value = { plc: plcId.value, id: connection.value, values: { ...values.value } }
}

// After the editor saved a connection, or deleted one, the list shows it as it is now.
watch(savedConnection, async (saved) => {
  if (!saved || saved.plc !== plcId.value) return
  await loadConnections()
  if (saved.id) {
    connection.value = saved.id
    // The form keeps what was filled in to edit it, once the new connection's form is up.
    await nextTick()
    if (saved.values) values.value = { ...values.value, ...saved.values }
  }
  savedConnection.value = null
})

const finding = ref(false)

// Looks for the variable list on the server the form names, and fills in where it is.
async function find() {
  finding.value = true
  message.value = ''
  try {
    values.value = { ...values.value, ...((await Service.FindCODESYSList(plcId.value, values.value)) ?? {}) }
  } catch (error) {
    message.value = (error as Error).message
  } finally {
    finding.value = false
  }
}
const link = computed(() => plcView.value?.link ?? null)

const pending = computed(() => {
  if (mode.value !== applied.value.mode) return true
  return mode.value === 'external' && (connection.value !== applied.value.connection || JSON.stringify(values.value) !== applied.value.values)
})

const STATES: { [state: string]: string } = { connected: 'Connected', connecting: 'Connecting', failed: 'Not connected' }

function setValue(f: Field, event: Event) {
  const el = event.target as HTMLInputElement | HTMLSelectElement
  let v: unknown = el.value
  if (f.options) v = f.options[Number(el.value)]?.value
  else if (f.type === 'boolean') v = (el as HTMLInputElement).checked
  else if (f.type === 'integer' || f.type === 'number') v = el.value === '' ? undefined : Number(el.value)
  const next = { ...values.value }
  if (v === undefined) delete next[f.name]
  else next[f.name] = v
  values.value = next
}

async function apply() {
  busy.value = true
  message.value = ''
  const external = mode.value === 'external'
  const config = { mode: mode.value, connection: external ? connection.value : '', values: external ? values.value : {} }
  try {
    await Service.SetPLCFor(plcId.value, config)
    applied.value = { mode: config.mode, connection: config.connection, values: JSON.stringify(config.values) }
    if (external) storeValues(config.connection, config.values, fields.value)
  } catch (error) {
    message.value = (error as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="section connection">
    <h3 class="section-title">PLC connection</h3>
    <div class="modes" role="radiogroup" aria-label="PLC connection">
      <button v-for="m in MODES" :key="m.id" role="radio" :aria-checked="mode === m.id" :class="{ active: mode === m.id }" @click="mode = m.id">
        {{ m.label }}
      </button>
    </div>
    <p class="help">{{ help }}</p>
    <template v-if="mode === 'external'">
      <label class="field">
        <span>Connection</span>
        <select v-model="connection" @focus="loadConnections">
          <optgroup v-for="source in sources" :key="source" :label="source">
            <option v-for="c in connections.filter((x) => x.source === source)" :key="c.id" :value="c.id">{{ c.name }}</option>
          </optgroup>
        </select>
      </label>
      <p v-if="chosen?.description" class="help">{{ chosen.description }}</p>
      <template v-for="f in fields" :key="`${connection}.${f.name}`">
        <label v-if="f.type === 'boolean'" class="check">
          <input type="checkbox" :checked="values[f.name] === true" :disabled="f.readOnly" @change="setValue(f, $event)" />
          <span>{{ f.title }}</span>
        </label>
        <label v-else class="field" :title="f.description">
          <span>{{ f.title }}<template v-if="f.required"> *</template></span>
          <select v-if="f.options" :disabled="f.readOnly" @change="setValue(f, $event)">
            <option v-for="(o, i) in f.options" :key="i" :value="i" :selected="o.value === values[f.name]">{{ o.label }}</option>
          </select>
          <input
            v-else
            class="mono"
            :type="f.secret ? 'password' : f.type === 'integer' || f.type === 'number' ? 'number' : 'text'"
            :value="values[f.name] ?? ''"
            :min="f.min"
            :max="f.max"
            :step="f.step"
            :placeholder="f.placeholder"
            :disabled="f.readOnly"
            spellcheck="false"
            autocomplete="off"
            @change="setValue(f, $event)"
          />
          <small v-if="f.description" class="hint">{{ f.description }}</small>
        </label>
      </template>
      <div class="actions">
        <button
          v-if="connection.startsWith('codesys')"
          :disabled="finding"
          title="Looks for the variable list on the server and fills in where it is"
          @click="find"
        >
          {{ finding ? 'Looking…' : 'Find on the server' }}
        </button>
        <button :disabled="!connection" title="Which node each signal is exchanged with" @click="editBindings">Edit bindings</button>
      </div>
    </template>
    <button v-if="pending" class="apply" :disabled="busy || (mode === 'external' && !connection)" @click="apply">
      {{ mode === 'none' ? 'Disconnect the PLC' : 'Connect' }} and restart the station
    </button>
    <p v-if="message" class="error" role="alert">{{ message }}</p>
    <div v-if="link && !pending" class="status" :class="link.state" role="status">
      <p class="state">
        <span class="lamp" />{{ STATES[link.state] ?? link.state }}
        <span v-if="link.state === 'connected'" class="mono numbers">{{ link.variables }} signals · {{ link.roundTrip.toFixed(1) }} ms</span>
      </p>
      <p class="mono address">{{ link.endpoint }}</p>
      <p v-if="link.detail" class="detail">{{ link.detail }}</p>
      <details v-if="link.problems?.length" class="problems" open>
        <summary>{{ link.problems.length }} {{ link.problems.length === 1 ? 'signal' : 'signals' }} cannot be exchanged</summary>
        <ul>
          <li v-for="p in link.problems" :key="p" class="mono">{{ p }}</li>
        </ul>
      </details>
      <details v-if="link.unbound?.length" class="problems unbound">
        <summary>{{ link.unbound.length }} {{ link.unbound.length === 1 ? 'signal is' : 'signals are' }} not part of the connection</summary>
        <ul>
          <li v-for="u in link.unbound" :key="u" class="mono">{{ u }}</li>
        </ul>
      </details>
    </div>
  </section>
</template>

<style scoped>
.modes {
  display: grid;
  grid-auto-flow: column;
  grid-auto-columns: 1fr;
  margin-bottom: 8px;
}

.modes button {
  border-radius: 0;
  margin-left: -1px;
}

.modes button:first-child {
  margin-left: 0;
}

.modes .active {
  background: var(--accent);
  border-color: var(--accent);
  color: #1b1405;
  font-weight: 600;
  position: relative;
}

.help,
.detail {
  color: var(--text-dim);
  margin: 0 0 8px;
}

.field {
  display: grid;
  gap: 3px;
  margin-bottom: 8px;
}

.field span {
  color: var(--text-dim);
  font-size: 12px;
}

.field input,
.field select {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 2px;
  color: var(--text);
  padding: 5px 7px;
  width: 100%;
}

.apply {
  margin-bottom: 8px;
  width: 100%;
}

.error {
  color: var(--danger);
  margin: 0 0 8px;
}

.status {
  border: 1px solid var(--line);
  padding: 8px;
}

.status p {
  margin: 0;
}

.state {
  align-items: center;
  display: flex;
  font-weight: 600;
  gap: 7px;
}

.numbers {
  color: var(--text-dim);
  font-weight: 400;
  margin-left: auto;
}

.lamp {
  background: var(--accent);
  border-radius: 50%;
  height: 9px;
  width: 9px;
}

.connected .lamp {
  background: #4ee67c;
}

.failed .lamp {
  background: var(--danger);
}

.address {
  color: var(--text-dim);
  font-size: 11px;
  margin-top: 3px;
  overflow-wrap: anywhere;
}

.detail {
  margin: 6px 0 0;
}

.failed .detail {
  color: var(--danger);
}

.actions {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}

.actions button {
  flex: 1;
}

.check {
  align-items: center;
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}

.hint {
  color: var(--text-dim);
  font-size: 11px;
}

.problems {
  margin-top: 6px;
}

.problems summary {
  color: var(--danger);
  cursor: pointer;
}

.unbound summary {
  color: var(--text-dim);
}

.problems ul {
  margin: 4px 0 0;
  padding-left: 16px;
}

.problems li {
  font-size: 11px;
  overflow-wrap: anywhere;
}
</style>
