<script setup lang="ts">
import { computed } from 'vue'
import type { BuilderParam } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'

// One setting of a module in the builder: on or off, one of its options, or a number within
// its limits, with what it does said below it.
const props = defineProps<{ param: BuilderParam; busy: boolean }>()
const emit = defineEmits<{ set: [value: string] }>()

const current = computed(() => props.param.value || props.param.default)
const on = computed(() => ['true', '1'].includes(current.value))
// A value the station writes as an expression cannot be picked from a list.
const pickable = computed(() => (props.param.options ?? []).length > 0 && (props.param.options ?? []).some((o) => String(o) === current.value))
const name = (index: number) => props.param.names?.[index] ?? `${props.param.options?.[index]} ${props.param.unit ?? ''}`.trim()
const limits = computed(() => {
  const p = props.param
  return p.min != null && p.max != null ? `${p.min} to ${p.max}${p.unit ? ' ' + p.unit : ''}` : ''
})
</script>

<template>
  <div class="setting">
    <label v-if="param.flag" class="field flag">
      <input type="checkbox" :checked="on" :disabled="busy" @change="emit('set', ($event.target as HTMLInputElement).checked ? 'true' : 'false')" />
      <span>{{ param.label }}</span>
      <button v-if="param.value" class="reset" title="Back to how it comes" @click.prevent="emit('set', '')">↺</button>
    </label>
    <label v-else class="field">
      <span>
        {{ param.label }}<small v-if="param.unit"> {{ param.unit }}</small>
        <button v-if="param.value" class="reset" title="Back to how it comes" @click.prevent="emit('set', '')">↺</button>
      </span>
      <select v-if="pickable" :value="current" :disabled="busy" @change="emit('set', ($event.target as HTMLSelectElement).value)">
        <option v-for="(option, index) in param.options" :key="option" :value="String(option)">{{ name(index) }}</option>
      </select>
      <input v-else :value="param.value" :placeholder="param.default" :title="limits" :disabled="busy" @change="emit('set', ($event.target as HTMLInputElement).value)" />
    </label>
    <p v-if="param.description" class="about">
      {{ param.description }}<template v-if="limits && !pickable"> From {{ limits }}.</template>
    </p>
  </div>
</template>

<style scoped>
.field {
  display: grid;
  font-size: 13px;
  gap: 3px;
  margin-top: 8px;
}

.field span {
  color: var(--text-dim);
}

.field small {
  margin-left: 4px;
}

.field input,
select {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 2px;
  color: var(--text);
  font: inherit;
  padding: 4px 6px;
}

.field.flag {
  align-items: center;
  display: flex;
  gap: 8px;
}

.field.flag span {
  color: var(--text);
}

.reset {
  background: none;
  border: 0;
  color: var(--accent);
  cursor: pointer;
  font-size: 12px;
  padding: 0 4px;
}

.about {
  color: var(--text-dim);
  font-size: 11.5px;
  line-height: 1.4;
  margin: 3px 0 0;
}
</style>
