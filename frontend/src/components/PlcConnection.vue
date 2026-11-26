<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { ConnectionInfo, PLCConfig } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { changedSince, connectionsChanged, editingConnection, plcStates, refreshPlcs, savedConnection, simulation } from '../sim'
import UiSegmented from '../ui/UiSegmented.vue'
import UiSelect from '../ui/UiSelect.vue'
import UiSwitch from '../ui/UiSwitch.vue'
import IconSearch from '~icons/fluent/search-16-regular'
import IconBindings from '~icons/fluent/arrow-swap-16-regular'
import UiField from '../ui/UiField.vue'

// How a PLC of the station connects, in the simulation's panel with the PLC selected: none, the
// app's soft PLC, or a controller outside the app. Choosing takes effect at once and is kept in
// the project: None lets go of the PLC, and another mode connects it again in that mode if it was
// connected. The PLC's card on the left connects and disconnects it, and offers Reconnect when
// the form changed while it runs. An external PLC keeps a connection of each kind it was given,
// so that trying another kind loses nothing; its bindings are edited in a dialog over the
// simulation. What its connection cannot exchange shows here. The mode goes to the parent, which
// shows the program only for the soft PLC.
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
    running.value = r
    plcStates.value = { ...plcStates.value, [plcId.value]: { planned: p, running: r } }
    kinds.value = k ?? []
    values.value = { ...defaultsOf(varsText(p.external?.connection?.vars)), ...p.external?.values }
  } catch (error) {
    message.value = (error as Error).message
  }
}

// The mode shown is the one the project has for the PLC, which is the one that runs unless
// connecting in it failed.
async function show() {
  await load()
  mode.value = planned.value?.mode ?? 'none'
}
onMounted(show)

// A connection put back as the project's original has it, or changed elsewhere, shows as it is.
watch(connectionsChanged, () => {
  justSaved.value = false
  void show()
})

const runs = computed(() => (running.value?.mode ?? 'none') !== 'none')

// Connects the PLC again as the project now has it, if it was connected; what fails lets go of
// it, saying why, for the card's Connect once the form is right.
async function reconnect() {
  if (!runs.value) return
  try {
    await Service.ConnectPLCFor(plcId.value)
  } catch (error) {
    await Service.DisconnectPLCFor(plcId.value).catch(() => undefined)
    message.value = (error as Error).message
  }
}

// Chooses the mode, at once: None lets go of the PLC, another mode is kept and, if the PLC was
// connected, connects it in that mode.
async function setMode(m: string) {
  if (m === (planned.value?.mode ?? 'none')) return
  mode.value = m
  busy.value = true
  message.value = ''
  justSaved.value = false
  window.clearTimeout(valuesTimer)
  try {
    if (m === 'none') await Service.SetPLCFor(plcId.value, { mode: m })
    else {
      if (m === 'external' && external.value) await Service.SetConnectionValues(plcId.value, values.value)
      await Service.PlanPLCFor(plcId.value, m)
      await reconnect()
    }
  } catch (error) {
    message.value = (error as Error).message
  } finally {
    await load()
    mode.value = planned.value?.mode ?? 'none'
    busy.value = false
  }
}
const chosenMode = computed({ get: () => mode.value, set: (m: string) => void setMode(m) })

// Gives the PLC its connection of a kind: the one it had of that kind, form and bindings and
// all, or a new one, and connects it again with it if it was connected. The connection it leaves
// is kept, so nothing needs asking.
async function chooseKind(id: string | null) {
  if (!id || id === external.value?.kind) return
  window.clearTimeout(valuesTimer)
  message.value = ''
  try {
    if (external.value) await Service.SetConnectionValues(plcId.value, values.value)
    await Service.ChooseConnection(plcId.value, id)
    await reconnect()
  } catch (error) {
    message.value = (error as Error).message
  }
  await load()
}

// What the form has is kept in the project as it is typed, such as the server's address.
let valuesTimer = 0
function keepValues() {
  window.clearTimeout(valuesTimer)
  valuesTimer = window.setTimeout(async () => {
    try {
      planned.value = await Service.SetConnectionValues(plcId.value, values.value)
      void refreshPlcs()
    } catch (error) {
      message.value = (error as Error).message
    }
  }, 300)
}

const help = computed(() => MODES.find((m) => m.id === mode.value)?.help)

// Whether the PLC connects as the project opens, in the mode chosen. Everyone
// may switch it, and it is kept in the project, so it goes with the project when handed out.
async function setAutoConnect(on: boolean) {
  message.value = ''
  try {
    planned.value = await Service.SetAutoConnect(plcId.value, on)
    void refreshPlcs()
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
    justSaved.value = runs.value
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

// The connection changed since the PLC connected, which the card's Reconnect takes.
const changed = computed(() => changedSince(plcStates.value[plcId.value]))

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
</script>

<template>
  <section class="t-section">
    <h3 class="t-section-title">Connection</h3>
    <UiSegmented v-model="chosenMode" label="PLC connection" :options="MODES.map((m) => ({ value: m.id, label: m.label, disabled: busy }))" />
    <p class="m-0 mt-2 text-xs text-muted">{{ help }}</p>
    <UiSwitch
      v-if="mode !== 'none'"
      class="mt-3"
      trailing
      label="Connect when the project opens"
      :model-value="!!planned?.autoConnect"
      @update:model-value="(on: boolean) => void setAutoConnect(on)"
    />
    <p v-if="message" class="m-0 mt-2 text-danger" role="alert">{{ message }}</p>

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
          :disabled="f.readOnly"
          :model-value="chosenOption(f)"
          @update:model-value="(i: string | null) => i !== null && setValue(f, f.options?.[Number(i)]?.value)"
        />
        <label v-else class="flex flex-col gap-1" :title="f.description">
          <span class="t-label">{{ f.title }}<template v-if="f.required"> *</template></span>
          <UiField
            class="t-input font-mono text-xs select-text"
            :type="f.secret ? 'password' : f.type === 'integer' || f.type === 'number' ? 'number' : 'text'"
            :value="String(values[f.name] ?? '')"
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
      <p v-if="changed" class="m-0 text-xs text-accent-ink" role="status">
        {{ justSaved ? 'The bindings are saved.' : 'Changed since it connected.' }} Reconnect on the PLC's card on the left to use it.
      </p>
    </div>

    <div v-if="link && runs && (link.detail || link.problems?.length || link.unbound?.length)" class="mt-3 flex flex-col gap-1.5">
      <p class="m-0 font-mono text-[11px] break-all text-muted">{{ link.endpoint }}</p>
      <p v-if="link.detail" class="m-0 text-xs" :class="link.state === 'failed' ? 'text-danger' : 'text-muted'">{{ link.detail }}</p>
      <details v-if="link.problems?.length" open>
        <summary class="cursor-default text-xs font-medium text-danger">
          {{ link.problems.length }} {{ link.problems.length === 1 ? 'signal' : 'signals' }} cannot be exchanged
        </summary>
        <ul class="m-0 mt-1 pl-4 font-mono text-[11px] text-muted">
          <li v-for="p in link.problems" :key="p">{{ p }}</li>
        </ul>
      </details>
      <details v-if="link.unbound?.length">
        <summary class="cursor-default text-xs font-medium text-muted">
          {{ link.unbound.length }} {{ link.unbound.length === 1 ? 'signal is' : 'signals are' }} not part of the connection
        </summary>
        <ul class="m-0 mt-1 pl-4 font-mono text-[11px] text-muted">
          <li v-for="u in link.unbound" :key="u">{{ u }}</li>
        </ul>
      </details>
    </div>
  </section>
</template>
