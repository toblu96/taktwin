<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { ConnectionInfo, PLCConfig } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { ask } from '../ask'
import { connectionsChanged, editingConnection, savedConnection, simulation } from '../sim'
import UiSegmented from '../ui/UiSegmented.vue'
import UiSelect from '../ui/UiSelect.vue'
import UiSwitch from '../ui/UiSwitch.vue'
import IconSearch from '~icons/fluent/search-16-regular'
import IconBindings from '~icons/fluent/arrow-swap-16-regular'
import IconDisconnect from '~icons/fluent/plug-disconnected-16-regular'
import IconChange from '~icons/fluent/arrow-repeat-all-16-regular'

// How a PLC of the station is connected: none, the app's soft PLC, or a controller outside the
// app, and how the connection is doing. An external PLC has one connection, the project's own:
// made once from a kind, such as CODESYS or FluidSIM, then filled in and bound in the project,
// and kept while another mode runs. It is the PLC given by its index, or else the one the
// selection picks.
const props = defineProps<{ index?: number }>()
const plcId = computed(() => (props.index === undefined ? simulation.plcId.value : props.index > 0 ? (simulation.scene.value?.plcs?.[props.index]?.module ?? '') : ''))
const plcView = computed(() => (props.index === undefined ? simulation.plcView.value : (simulation.snapshot.value?.plcs?.[props.index] ?? null)))

const MODES = [
  { id: 'none', label: 'None', help: 'No PLC: every output stays off. Switch them one by one with the I/O test, or connect a PLC.' },
  { id: 'soft', label: 'Soft PLC', help: "The app's own PLC, running the Structured Text program written in its editor, reached over OPC UA as any other PLC." },
  {
    id: 'external',
    label: 'External',
    help: "The station connects to a controller outside the app, such as a CODESYS PLC or FluidSIM, through the project's own connection.",
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

const mode = ref('none')
const kinds = ref<ConnectionInfo[]>([])
// How the project means to connect the PLC, with its own connection, and how it runs now.
const planned = ref<PLCConfig | null>(null)
const running = ref<PLCConfig | null>(null)
const values = ref<{ [name: string]: unknown }>({})
const message = ref('')
const busy = ref(false)
// Choosing a kind shows while there is no connection yet, or to replace it.
const choosing = ref(false)

const external = computed(() => planned.value?.external ?? null)
const kind = computed(() => kinds.value.find((k) => k.id === external.value?.kind) ?? null)
const varsText = (vars: unknown) => (vars ? JSON.stringify(vars) : '')
const fields = computed(() => fieldsOf(varsText(external.value?.connection?.vars)))

async function load() {
  message.value = ''
  try {
    const [p, r, k] = await Promise.all([Service.PlannedPLC(plcId.value), Service.PLCFor(plcId.value), Service.ConnectionKinds(plcId.value)])
    planned.value = p
    running.value = r
    kinds.value = k ?? []
    values.value = { ...defaultsOf(varsText(p.external?.connection?.vars)), ...p.external?.values }
    choosing.value = !p.external
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

// Gives the PLC a new connection of a kind, in place of the one it had, after asking.
async function chooseKind(id: string | null) {
  if (!id) return
  const k = kinds.value.find((x) => x.id === id)
  if (external.value && !(await ask(`Replace the PLC's connection, with its bindings and what its form has, by a new one of the kind ${k?.name ?? id}?`, 'Replace'))) return
  try {
    await Service.NewConnection(plcId.value, id)
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

function editBindings() {
  editingConnection.value = { plc: plcId.value, values: { ...values.value } }
}

// After the bindings were saved, connecting again makes the PLC exchange with them.
const justSaved = ref(false)
watch(savedConnection, async (saved) => {
  if (!saved || saved.plc !== plcId.value) return
  await load()
  justSaved.value = true
  savedConnection.value = null
})

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

const STATES: { [state: string]: string } = { connected: 'Connected', connecting: 'Connecting', failed: 'Not connected' }

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
const kindOptions = computed(() => kinds.value.map((c) => ({ value: c.id, label: c.name, hint: c.source })))

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
    <UiSegmented v-model="mode" label="PLC connection" :options="MODES.map((m) => ({ value: m.id, label: m.label }))" />
    <p class="m-0 mt-2 text-xs text-muted">{{ help }}</p>

    <div v-if="mode === 'external'" class="mt-3 flex flex-col gap-3">
      <template v-if="choosing">
        <UiSelect
          :model-value="null"
          label="Kind of connection"
          :options="kindOptions"
          placeholder="Choose what the PLC is"
          @update:model-value="(id: string | null) => void chooseKind(id)"
        />
        <p class="m-0 -mt-1 text-xs text-muted">
          {{ external ? 'A new connection replaces the one the project has, bindings and all.' : 'The project then keeps the connection as its own, to fill in and bind.' }}
        </p>
        <button v-if="external" class="t-btn self-start" @click="choosing = false">Keep the connection</button>
      </template>
      <div v-else-if="external" class="flex items-start gap-2">
        <div class="min-w-0 flex-1">
          <p class="m-0 font-medium">{{ external.connection.name || kind?.name || external.kind }}</p>
          <p v-if="external.connection.description" class="m-0 mt-0.5 text-xs text-muted">{{ external.connection.description }}</p>
        </div>
        <button class="t-btn h-6 shrink-0 px-2 text-xs" title="Start a new connection of another kind" @click="choosing = true"><IconChange class="size-3.5" />Change…</button>
      </div>
      <template v-for="f in choosing ? [] : fields" :key="`${external?.kind}.${f.name}`">
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
          <span v-if="f.description" class="text-xs text-muted">{{ f.description }}</span>
        </label>
      </template>
      <div class="flex flex-wrap gap-2">
        <button
          v-if="!choosing && external?.kind === 'codesys'"
          class="t-btn"
          :disabled="finding"
          title="Look for the variable list on the server and fill in where it is"
          @click="find"
        >
          <IconSearch class="size-4" />{{ finding ? 'Looking…' : 'Find on the server' }}
        </button>
        <button v-if="!choosing && external" class="t-btn" title="Which node each signal is exchanged with" @click="editBindings">
          <IconBindings class="size-4" />Edit bindings
        </button>
      </div>
    </div>

    <p v-if="pending && justSaved" class="m-0 mt-3 text-xs text-accent-ink" role="status">The bindings are saved. Connect to exchange with them.</p>
    <button v-if="pending" class="t-btn t-btn-primary mt-3 w-full justify-center" :disabled="busy || (mode === 'external' && !external)" @click="apply">
      {{ mode === 'none' ? 'Disconnect the PLC' : 'Connect' }} and restart the station
    </button>
    <p v-if="message" class="m-0 mt-2 text-danger" role="alert">{{ message }}</p>

    <div v-if="link && !pending" class="mt-3 rounded-control border border-line bg-s0 px-2.5 py-2" role="status">
      <p class="m-0 flex items-center gap-2">
        <span class="size-2 shrink-0 rounded-full" :class="{ connected: 'bg-ok', connecting: 'bg-accent', failed: 'bg-danger' }[link.state] ?? 'bg-faint'" />
        <span class="flex-1 font-medium">{{ STATES[link.state] ?? link.state }}</span>
        <span v-if="link.state === 'connected'" class="font-mono text-[11px] text-muted">{{ link.variables }} signals · {{ link.roundTrip.toFixed(1) }} ms</span>
      </p>
      <p class="m-0 mt-1 truncate font-mono text-[11px] text-muted" :title="link.endpoint">{{ link.endpoint }}</p>
      <p v-if="link.detail" class="m-0 mt-1 text-xs" :class="link.state === 'failed' ? 'text-danger' : 'text-muted'">{{ link.detail }}</p>
      <details v-if="link.problems?.length" class="mt-2" open>
        <summary class="cursor-default text-xs font-medium text-danger">
          {{ link.problems.length }} {{ link.problems.length === 1 ? 'signal' : 'signals' }} cannot be exchanged
        </summary>
        <ul class="m-0 mt-1 pl-4 font-mono text-[11px] text-muted">
          <li v-for="p in link.problems" :key="p">{{ p }}</li>
        </ul>
      </details>
      <details v-if="link.unbound?.length" class="mt-2">
        <summary class="cursor-default text-xs font-medium text-muted">
          {{ link.unbound.length }} {{ link.unbound.length === 1 ? 'signal is' : 'signals are' }} not part of the connection
        </summary>
        <ul class="m-0 mt-1 pl-4 font-mono text-[11px] text-muted">
          <li v-for="u in link.unbound" :key="u">{{ u }}</li>
        </ul>
      </details>
    </div>
    <button
      v-if="(running?.mode ?? 'none') !== 'none' && !pending"
      class="t-btn mt-3"
      :disabled="busy"
      title="Let go of the PLC, keeping its connection to connect again"
      @click="disconnect"
    >
      <IconDisconnect class="size-4" />Disconnect
    </button>
  </section>
</template>
