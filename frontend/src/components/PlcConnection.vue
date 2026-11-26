<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { ConnectionInfo, PLCConfig } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { connectionsChanged, editingConnection, plans, savedConnection, simulation } from '../sim'
import UiSegmented from '../ui/UiSegmented.vue'
import UiSelect from '../ui/UiSelect.vue'
import UiSwitch from '../ui/UiSwitch.vue'
import IconSearch from '~icons/fluent/search-16-regular'
import IconBindings from '~icons/fluent/arrow-swap-16-regular'
import IconConnect from '~icons/fluent/plug-connected-16-regular'
import IconDisconnect from '~icons/fluent/plug-disconnected-16-regular'

// How a PLC of the station is connected, in the simulation's panel with the PLC selected. On top
// is how the connection is doing, with Connect or Disconnect always in the same place; below it
// the mode: none, the app's soft PLC, or a controller outside the app. An external PLC keeps a
// connection of each kind it was given, so that trying another kind loses nothing; its bindings
// are edited in a dialog over the simulation. The mode chosen goes to the parent, which shows
// the program only for the soft PLC.
const { plcId, plcView } = simulation
const shownMode = defineModel<string>('mode', { default: 'none' })

const MODES = [
  { id: 'none', label: 'None', help: 'No PLC: every output stays off.' },
  { id: 'soft', label: 'Soft PLC', help: "The app's own PLC, running the program written in its editor." },
  { id: 'external', label: 'External', help: 'A controller outside the app, such as CODESYS or FluidSIM, over OPC UA.' },
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

const mode = ref('none')
watch(mode, (m) => (shownMode.value = m), { immediate: true })
const kinds = ref<ConnectionInfo[]>([])
// How the project means to connect the PLC, with its own connection, and how it runs now.
const planned = ref<PLCConfig | null>(null)
const running = ref<PLCConfig | null>(null)
const values = ref<{ [name: string]: unknown }>({})
const message = ref('')
const busy = ref(false)

const external = computed(() => planned.value?.external ?? null)
const kind = computed(() => kinds.value.find((k) => k.id === external.value?.kind) ?? null)
const varsText = (vars: unknown) => (vars ? JSON.stringify(vars) : '')
const fields = computed(() => fieldsOf(varsText(external.value?.connection?.vars)))

async function load() {
  message.value = ''
  try {
    const [p, r, k] = await Promise.all([Service.PlannedPLC(plcId.value), Service.PLCFor(plcId.value), Service.ConnectionKinds(plcId.value)])
    planned.value = p
    plans.value = { ...plans.value, [plcId.value]: p }
    running.value = r
    kinds.value = k ?? []
    values.value = { ...defaultsOf(varsText(p.external?.connection?.vars)), ...p.external?.values }
  } catch (error) {
    message.value = (error as Error).message
  }
}

// The mode shown is the one that runs, or for a PLC not connected yet what the project has for
// it, ready to connect.
async function show() {
  await load()
  mode.value = running.value?.mode !== 'none' ? (running.value?.mode ?? 'none') : (planned.value?.mode ?? 'none')
}
onMounted(show)

// A connection put back as the project's original has it, or changed elsewhere, shows as it is.
watch(connectionsChanged, () => {
  justSaved.value = false
  void show()
})

// Gives the PLC its connection of a kind: the one it had of that kind, form and bindings and
// all, or a new one. The connection it leaves is kept, so nothing needs asking.
async function chooseKind(id: string | null) {
  if (!id || id === external.value?.kind) return
  window.clearTimeout(valuesTimer)
  try {
    if (external.value) await Service.SetConnectionValues(plcId.value, values.value)
    await Service.ChooseConnection(plcId.value, id)
    await load()
  } catch (error) {
    message.value = (error as Error).message
  }
}

// What the form has is kept in the project as it is typed, such as the server's address.
let valuesTimer = 0
function keepValues() {
  window.clearTimeout(valuesTimer)
  valuesTimer = window.setTimeout(async () => {
    try {
      planned.value = await Service.SetConnectionValues(plcId.value, values.value)
    } catch (error) {
      message.value = (error as Error).message
    }
  }, 300)
}

const help = computed(() => MODES.find((m) => m.id === mode.value)?.help)

// Whether the PLC connects as the project opens, in the mode it was last connected in. Everyone
// may switch it, and it is kept in the project, so it goes with the project when handed out.
async function setAutoConnect(on: boolean) {
  message.value = ''
  try {
    const p = await Service.SetAutoConnect(plcId.value, on)
    planned.value = p
    plans.value = { ...plans.value, [plcId.value]: p }
  } catch (error) {
    message.value = (error as Error).message
  }
}

// The bindings open in a dialog over the simulation, with what the form has typed so far.
function editBindings() {
  window.clearTimeout(valuesTimer)
  editingConnection.value = { plc: plcId.value, values: { ...values.value } }
}

// After the bindings were saved, connecting again makes the PLC exchange with them.
const justSaved = ref(false)
watch(
  savedConnection,
  async (saved) => {
    if (!saved || saved.plc !== plcId.value) return
    savedConnection.value = null
    await load()
    justSaved.value = true
  },
  { immediate: true },
)

const finding = ref(false)

// Looks for the variable list on the server the form names, and fills in where it is.
async function find() {
  finding.value = true
  message.value = ''
  try {
    values.value = { ...values.value, ...(await Service.FindCODESYSList(plcId.value, values.value)) }
    keepValues()
  } catch (error) {
    message.value = (error as Error).message
  } finally {
    finding.value = false
  }
}
const link = computed(() => plcView.value?.link ?? null)

const pending = computed(() => {
  if (mode.value !== (running.value?.mode ?? 'none')) return true
  return mode.value === 'external' && JSON.stringify(planned.value?.external ?? null) !== JSON.stringify(running.value?.external ?? null)
})

const STATES: { [state: string]: string } = { connected: 'Connected', connecting: 'Connecting…', failed: 'Not connected' }
const LAMPS: { [state: string]: string } = { connected: 'bg-ok', connecting: 'bg-accent', failed: 'bg-danger' }

// How the connection is doing, in two lines that are always there: what runs, and where it is
// connected to with how many signals, so that connecting changes no heights.
const state = computed(() => {
  const runs = running.value?.mode ?? 'none'
  if (runs === 'none') return { label: 'Not connected', lamp: 'bg-faint', detail: 'Every output stays off' }
  const l = link.value
  if (!l) return { label: 'Running', lamp: 'bg-ok', detail: runs === 'soft' ? 'Soft PLC' : '' }
  const numbers = l.state === 'connected' ? ` · ${l.variables} signals · ${l.roundTrip.toFixed(1)} ms` : ''
  return { label: STATES[l.state] ?? l.state, lamp: LAMPS[l.state] ?? 'bg-faint', detail: `${l.endpoint}${numbers}` }
})

// The card's button: Connect what is chosen, Disconnect what runs, or Disconnect greyed out when
// nothing runs and nothing is chosen. Both are as wide, so swapping them moves nothing.
const action = computed(() => {
  if (pending.value && mode.value !== 'none') return 'connect'
  return (running.value?.mode ?? 'none') === 'none' ? 'none' : 'disconnect'
})

// A field of the form changed: typed, chosen from its list, or switched.
function typed(f: Field, event: Event) {
  const text = (event.target as HTMLInputElement).value
  setValue(f, f.type === 'integer' || f.type === 'number' ? (text === '' ? undefined : Number(text)) : text)
}
const optionsOf = (f: Field) => (f.options ?? []).map((o, i) => ({ value: String(i), label: o.label }))
const chosenOption = (f: Field) => {
  const i = f.options?.findIndex((o) => o.value === values.value[f.name]) ?? -1
  return i < 0 ? null : String(i)
}
const kindOptions = computed(() => kinds.value.map((c) => ({ value: c.id, label: c.name, description: c.description })))

function setValue(f: Field, v: unknown) {
  const next = { ...values.value }
  if (v === undefined) delete next[f.name]
  else next[f.name] = v
  values.value = next
  keepValues()
}

// Lets go of the PLC that runs, also one that cannot be reached, and keeps its form filled in,
// so that Connect brings it back.
async function disconnect() {
  busy.value = true
  message.value = ''
  try {
    await Service.DisconnectPLCFor(plcId.value)
    await load()
  } catch (error) {
    message.value = (error as Error).message
  } finally {
    busy.value = false
  }
}

async function apply() {
  justSaved.value = false
  busy.value = true
  message.value = ''
  window.clearTimeout(valuesTimer)
  try {
    if (mode.value === 'external') await Service.SetConnectionValues(plcId.value, values.value)
    // The project's own connection goes along.
    await Service.SetPLCFor(plcId.value, { mode: mode.value })
    await load()
  } catch (error) {
    message.value = (error as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="t-section">
    <h3 class="t-section-title">Connection</h3>
    <div class="rounded-control border border-line bg-s0 px-2.5 py-2" role="status">
      <div class="flex items-center gap-2">
        <span class="size-2 shrink-0 rounded-full" :class="state.lamp" />
        <span class="min-w-0 flex-1 truncate font-medium">{{ state.label }}</span>
        <button
          v-if="action === 'connect'"
          class="t-btn t-btn-primary w-28 shrink-0 justify-center"
          :disabled="busy || (mode === 'external' && !external)"
          title="Connect, and restart the station"
          @click="apply"
        >
          <IconConnect class="size-4" />Connect
        </button>
        <button
          v-else
          class="t-btn w-28 shrink-0 justify-center"
          :disabled="busy || action === 'none'"
          title="Let go of the PLC, keeping how it connects"
          @click="mode === 'none' ? apply() : disconnect()"
        >
          <IconDisconnect class="size-4" />Disconnect
        </button>
      </div>
      <p class="m-0 mt-1 font-mono text-[11px] break-all text-muted">{{ state.detail }}</p>
      <p v-if="pending && justSaved" class="m-0 mt-1.5 text-xs text-accent-ink">The bindings are saved. Connect to exchange with them.</p>
      <p v-else-if="pending && (running?.mode ?? 'none') !== 'none'" class="m-0 mt-1.5 text-xs text-accent-ink">Changed: connect again to use it.</p>
      <p v-if="link?.detail && !pending" class="m-0 mt-1.5 text-xs" :class="link.state === 'failed' ? 'text-danger' : 'text-muted'">{{ link.detail }}</p>
      <details v-if="link?.problems?.length && !pending" class="mt-1.5" open>
        <summary class="cursor-default text-xs font-medium text-danger">
          {{ link.problems.length }} {{ link.problems.length === 1 ? 'signal' : 'signals' }} cannot be exchanged
        </summary>
        <ul class="m-0 mt-1 pl-4 font-mono text-[11px] text-muted">
          <li v-for="p in link.problems" :key="p">{{ p }}</li>
        </ul>
      </details>
      <details v-if="link?.unbound?.length && !pending" class="mt-1.5">
        <summary class="cursor-default text-xs font-medium text-muted">
          {{ link.unbound.length }} {{ link.unbound.length === 1 ? 'signal is' : 'signals are' }} not part of the connection
        </summary>
        <ul class="m-0 mt-1 pl-4 font-mono text-[11px] text-muted">
          <li v-for="u in link.unbound" :key="u">{{ u }}</li>
        </ul>
      </details>
    </div>
    <p v-if="message" class="m-0 mt-2 text-danger" role="alert">{{ message }}</p>

    <div class="mt-3">
      <UiSegmented v-model="mode" label="PLC connection" :options="MODES.map((m) => ({ value: m.id, label: m.label }))" />
      <p class="m-0 mt-2 text-xs text-muted">{{ help }}</p>
      <UiSwitch
        v-if="mode !== 'none'"
        class="mt-3"
        trailing
        label="Connect when the project opens"
        :model-value="!!planned?.autoConnect"
        @update:model-value="(on: boolean) => void setAutoConnect(on)"
      />
    </div>

    <div v-if="mode === 'external'" class="mt-3 flex flex-col gap-3">
      <div class="flex flex-col gap-1">
        <UiSelect
          :model-value="external?.kind ?? null"
          label="Kind of connection"
          :options="kindOptions"
          placeholder="Choose what the PLC is"
          @update:model-value="(id: string | null) => void chooseKind(id)"
        />
        <p v-if="kind?.description" class="m-0 text-xs text-muted">{{ kind.description }}</p>
      </div>
      <template v-for="f in fields" :key="`${external?.kind}.${f.name}`">
        <UiSwitch
          v-if="f.type === 'boolean'"
          trailing
          :label="f.title"
          :disabled="f.readOnly"
          :model-value="values[f.name] === true"
          @update:model-value="(on: boolean) => setValue(f, on)"
        />
        <UiSelect
          v-else-if="f.options"
          :label="`${f.title}${f.required ? ' *' : ''}`"
          :options="optionsOf(f)"
          :model-value="chosenOption(f)"
          @update:model-value="(i: string | null) => i !== null && setValue(f, f.options?.[Number(i)]?.value)"
        />
        <label v-else class="flex flex-col gap-1" :title="f.description">
          <span class="t-label">{{ f.title }}<template v-if="f.required"> *</template></span>
          <input
            class="t-input font-mono text-xs select-text"
            :type="f.secret ? 'password' : f.type === 'integer' || f.type === 'number' ? 'number' : 'text'"
            :value="values[f.name] ?? ''"
            :min="f.min"
            :max="f.max"
            :step="f.step"
            :placeholder="f.placeholder"
            :disabled="f.readOnly"
            spellcheck="false"
            autocomplete="off"
            @change="typed(f, $event)"
          />
        </label>
      </template>
      <div v-if="external" class="flex flex-wrap gap-2">
        <button v-if="external.kind === 'codesys'" class="t-btn" :disabled="finding" title="Look for the variable list on the server and fill in where it is" @click="find">
          <IconSearch class="size-4" />{{ finding ? 'Looking…' : 'Find on the server' }}
        </button>
        <button class="t-btn" title="Which node each signal is exchanged with" @click="editBindings"><IconBindings class="size-4" />Edit bindings</button>
      </div>
    </div>
  </section>
</template>
