<script setup lang="ts">
// A module's settings, faults and how it is set up, or those of the whole station: numbers in a
// field with their unit, choices in a list, and what is on or off as a switch.
import { computed, useId } from 'vue'
import { useSim } from '../sim'
import type { SceneFault, SceneParameter, SceneValue } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import UiSelect from '../ui/UiSelect.vue'
import UiSwitch from '../ui/UiSwitch.vue'

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

// A setting changed in its field, its switch or its list, kept within its range.
function set(parameter: SceneParameter, value: number) {
  if (value >= parameter.min && value <= parameter.max) void setParameter(parameter.id, value)
}
const typed = (parameter: SceneParameter, event: Event) => set(parameter, Number((event.target as HTMLInputElement).value))
const options = (parameter: SceneParameter) => (parameter.options ?? []).map((o, i) => ({ value: String(o), label: optionName(parameter, i) }))
const id = useId()

// What an option is called, such as Round for a body's shape.
const optionName = (parameter: SceneParameter, index: number) => parameter.names?.[index] ?? `${parameter.options?.[index]} ${parameter.unit ?? ''}`

const faultActive = (name: string) => snapshot.value?.faults?.includes(name) ?? false
const setFault = (name: string, on: boolean) => void service.SetFault(name, on)
</script>

<template>
  <section v-if="parameters.length || faults.length || values.length" class="t-section flex flex-col gap-5">
    <div v-for="group in groups" :key="group.title">
      <h3 class="t-section-title">{{ group.title }}</h3>
      <div class="flex flex-col gap-3">
        <div v-for="parameter in group.list" :key="parameter.id">
          <UiSwitch
            v-if="parameter.boolean"
            trailing
            :label="parameter.label"
            :model-value="parameterValue(parameter.id, parameter.value) >= 0.5"
            @update:model-value="(on: boolean) => set(parameter, Number(on))"
          />
          <div v-else class="flex items-center gap-3">
            <label :for="`${id}-${parameter.id}`" class="min-w-0 flex-1">
              {{ parameter.label }}<span v-if="owner(parameter)" class="block truncate text-xs text-muted">{{ owner(parameter) }}</span>
            </label>
            <UiSelect
              v-if="parameter.options?.length"
              class="w-40 shrink-0"
              hide-label
              :label="parameter.label"
              :options="options(parameter)"
              :model-value="String(parameterValue(parameter.id, parameter.value))"
              @update:model-value="(v: string | null) => v !== null && set(parameter, Number(v))"
            />
            <span v-else class="flex shrink-0 items-center gap-1.5">
              <input
                :id="`${id}-${parameter.id}`"
                type="number"
                class="t-input w-20 text-right font-mono text-xs select-text"
                :min="parameter.min"
                :max="parameter.max"
                :step="parameter.step || 1"
                :value="parameterValue(parameter.id, parameter.value)"
                @change="typed(parameter, $event)"
              />
              <span class="w-9 text-xs text-muted">{{ parameter.unit }}</span>
            </span>
          </div>
          <p v-if="parameter.description" class="m-0 mt-1 text-xs text-muted">{{ parameter.description }}</p>
        </div>
      </div>
    </div>
    <div v-if="faults.length">
      <h3 class="t-section-title">Faults</h3>
      <div class="flex flex-col gap-2.5">
        <UiSwitch
          v-for="fault in faults"
          :key="fault.name"
          trailing
          :label="fault.label"
          :model-value="faultActive(fault.name)"
          @update:model-value="(on: boolean) => setFault(fault.name, on)"
        />
      </div>
    </div>
    <div v-if="values.length">
      <h3 class="t-section-title">Set up as</h3>
      <div class="flex flex-col gap-1.5">
        <div v-for="value in values" :key="value.id" class="flex items-baseline justify-between gap-3">
          <span class="text-muted">{{ value.label }}</span>
          <span class="font-mono text-xs">{{ Number(value.value.toFixed(2)) }} {{ value.unit }}</span>
        </div>
      </div>
    </div>
  </section>
</template>
