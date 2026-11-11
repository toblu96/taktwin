<script setup lang="ts">
import { computed } from 'vue'
import { useSim } from '../sim'
import type { SceneFault, SceneParameter, SceneValue } from '../../bindings/github.com/toblu96/taktwin/internal/model'

const props = defineProps<{ parameters: SceneParameter[]; faults: SceneFault[]; values: SceneValue[]; named?: boolean }>()

const { parameterValue, scene, service, setParameter, snapshot } = useSim()

// Settings that are about how something is built, and those about how it behaves, as in the builder.
const build = computed(() => props.parameters.filter((p) => !p.advanced && p.kind === 'build'))
const physics = computed(() => props.parameters.filter((p) => !p.advanced && p.kind !== 'build'))
const groups = computed(() =>
  [
    { title: 'Build settings', list: build.value },
    { title: 'Physics', list: physics.value },
  ].filter((g) => g.list.length),
)

// In a list for the whole station, say what each setting belongs to.
const owner = (parameter: SceneParameter) => (props.named ? scene.value?.instances?.find((i) => i.id === parameter.module)?.name : undefined)

function change(parameter: SceneParameter, event: Event) {
  const input = event.target as HTMLInputElement | HTMLSelectElement
  const value = parameter.boolean ? Number((input as HTMLInputElement).checked) : Number(input.value)
  if (value >= parameter.min && value <= parameter.max) void setParameter(parameter.id, value)
}

// What an option is called, such as Round for a body's shape.
const optionName = (parameter: SceneParameter, index: number) => parameter.names?.[index] ?? `${parameter.options?.[index]} ${parameter.unit ?? ''}`

const faultActive = (name: string) => snapshot.value?.faults?.includes(name) ?? false
const setFault = (name: string, event: Event) => void service.SetFault(name, (event.target as HTMLInputElement).checked)
</script>

<template>
  <section v-if="parameters.length || faults.length || values.length" class="section">
    <template v-for="group in groups" :key="group.title">
      <h3 class="section-title">{{ group.title }}</h3>
      <div v-for="parameter in group.list" :key="parameter.id" class="setting">
        <label class="row">
          <span
            >{{ parameter.label }}<small v-if="owner(parameter)" class="owner">{{ owner(parameter) }}</small></span
          >
          <span class="field">
            <input v-if="parameter.boolean" type="checkbox" :checked="parameterValue(parameter.id, parameter.value) >= 0.5" @change="change(parameter, $event)" />
            <select v-else-if="parameter.options?.length" :value="parameterValue(parameter.id, parameter.value)" @change="change(parameter, $event)">
              <option v-for="(option, index) in parameter.options" :key="option" :value="option">{{ optionName(parameter, index) }}</option>
            </select>
            <template v-else>
              <input
                type="number"
                :min="parameter.min"
                :max="parameter.max"
                :step="parameter.step || 1"
                :value="parameterValue(parameter.id, parameter.value)"
                @change="change(parameter, $event)"
              />
              {{ parameter.unit }}
            </template>
          </span>
        </label>
        <p v-if="parameter.description" class="description">{{ parameter.description }}</p>
      </div>
    </template>
    <template v-if="faults.length">
      <h3 class="section-title">Faults</h3>
      <label v-for="fault in faults" :key="fault.name" class="row">
        <span>{{ fault.label }}</span>
        <input type="checkbox" :checked="faultActive(fault.name)" @change="setFault(fault.name, $event)" />
      </label>
    </template>
    <template v-if="values.length">
      <h3 class="section-title">Set up as</h3>
      <div v-for="value in values" :key="value.id" class="row read-only">
        <span>{{ value.label }}</span>
        <span class="mono">{{ Number(value.value.toFixed(2)) }} {{ value.unit }}</span>
      </div>
    </template>
  </section>
</template>

<style scoped>
.owner {
  color: var(--text-dim);
  display: block;
  font-size: 11px;
}

.section-title + .setting,
.section-title + .row {
  margin-top: 2px;
}

.setting + .section-title,
.row + .section-title {
  margin-top: 14px;
}

.setting {
  margin-bottom: 6px;
}

.description {
  color: var(--text-dim);
  font-size: 11.5px;
  line-height: 1.4;
  margin: 0 0 4px;
}

select {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 2px;
  color: var(--text);
  padding: 3px 4px;
}

.row {
  align-items: center;
  display: flex;
  gap: 8px;
  justify-content: space-between;
  min-height: 28px;
}

.read-only {
  color: var(--text-dim);
  min-height: 22px;
}

.field {
  color: var(--text-dim);
  white-space: nowrap;
}

input[type='number'] {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 2px;
  padding: 3px 6px;
  text-align: right;
  user-select: text;
  -webkit-user-select: text;
  width: 62px;
}

input[type='checkbox'] {
  accent-color: var(--accent);
  flex: none;
  height: 15px;
  margin: 0;
  width: 15px;
}
</style>
