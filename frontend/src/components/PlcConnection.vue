<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { simulation } from '../sim'

const { plcId, plcScene, plcView } = simulation

const MODES = [
  { id: 'none', label: 'None', help: 'No PLC: every output stays off. Switch them one by one with the I/O test, or connect a PLC.' },
  { id: 'soft', label: 'Soft PLC', help: "The app's own PLC, running the Structured Text program written in its editor, reached over OPC UA as CODESYS would be." },
  {
    id: 'external',
    label: 'External',
    help: 'The station connects over OPC UA to a PLC such as CODESYS. The PLC must offer the Bits list through its symbol configuration, without security.',
  },
]

function storedEndpoint() {
  try {
    return localStorage.getItem('taktwin.endpoint')
  } catch {
    return null
  }
}

const mode = ref('none')
const endpoint = ref(storedEndpoint() ?? 'opc.tcp://127.0.0.1:4840')
const addressMap = ref('')
const message = ref('')
const busy = ref(false)

// What is running, to tell a change that still needs applying.
const applied = ref({ mode: 'none', endpoint: '', map: '' })

onMounted(async () => {
  const plc = await Service.PLCFor(plcId.value)
  applied.value = { mode: plc.mode, endpoint: plc.endpoint, map: plc.map }
  mode.value = plc.mode
  if (plc.endpoint) endpoint.value = plc.endpoint
  addressMap.value = plc.map
})

const help = computed(() => MODES.find((m) => m.id === mode.value)?.help)
const maps = computed(() => plcScene.value?.maps ?? [])
const link = computed(() => plcView.value?.link ?? null)

const pending = computed(() => {
  if (mode.value !== applied.value.mode) return true
  return mode.value === 'external' && (endpoint.value !== applied.value.endpoint || addressMap.value !== applied.value.map)
})

const STATES: { [state: string]: string } = { connected: 'Connected', connecting: 'Connecting', failed: 'Not connected' }

async function apply() {
  busy.value = true
  message.value = ''
  const config = { mode: mode.value, endpoint: mode.value === 'external' ? endpoint.value.trim() : '', map: mode.value === 'external' ? addressMap.value : '' }
  try {
    await Service.SetPLCFor(plcId.value, config)
    applied.value = config
    if (mode.value === 'external') {
      try {
        localStorage.setItem('taktwin.endpoint', config.endpoint)
      } catch {
        // It only means typing the address again next time.
      }
    }
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
        <span>Server address</span>
        <input v-model="endpoint" class="mono" type="text" spellcheck="false" autocomplete="off" />
      </label>
      <label v-if="maps.length" class="field">
        <span>PLC program addresses</span>
        <select v-model="addressMap">
          <option value="">The station's own addresses</option>
          <option v-for="m in maps" :key="m.id" :value="m.id">{{ m.name }}</option>
        </select>
      </label>
    </template>
    <button v-if="pending" class="apply" :disabled="busy" @click="apply">{{ mode === 'none' ? 'Disconnect the PLC' : 'Connect' }} and restart the station</button>
    <p v-if="message" class="error" role="alert">{{ message }}</p>
    <div v-if="link && !pending" class="status" :class="link.state" role="status">
      <p class="state">
        <span class="lamp" />{{ STATES[link.state] ?? link.state }}
        <span v-if="link.state === 'connected'" class="mono numbers">{{ link.variables }} variables · {{ link.roundTrip.toFixed(1) }} ms</span>
      </p>
      <p class="mono address">{{ link.endpoint }}</p>
      <p v-if="link.detail" class="detail">{{ link.detail }}</p>
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
</style>
