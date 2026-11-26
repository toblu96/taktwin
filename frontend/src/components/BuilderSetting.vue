<script setup lang="ts">
import { computed, useId } from 'vue'
import type { BuilderParam } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import UiSelect from '../ui/UiSelect.vue'
import UiSwitch from '../ui/UiSwitch.vue'
import IconReset from '~icons/fluent/arrow-counterclockwise-16-regular'
import UiField from '../ui/UiField.vue'

// One setting of a module in the builder: on or off, one of its options, or a value within its
// limits, which may be an expression of the station, with what it does said below it. A setting
// changed from how the module comes can be put back.
const props = defineProps<{ param: BuilderParam; busy: boolean }>()
const emit = defineEmits<{ set: [value: string] }>()

const current = computed(() => props.param.value || props.param.default)
const on = computed(() => ['true', '1'].includes(current.value))
// A value the station writes as an expression cannot be picked from a list.
const pickable = computed(() => (props.param.options ?? []).length > 0 && (props.param.options ?? []).some((o) => String(o) === current.value))
const options = computed(() => (props.param.options ?? []).map((o, i) => ({ value: String(o), label: props.param.names?.[i] ?? `${o} ${props.param.unit ?? ''}`.trim() })))
const limits = computed(() => {
  const p = props.param
  return p.min != null && p.max != null ? `${p.min} to ${p.max}${p.unit ? ` ${p.unit}` : ''}` : ''
})
const id = useId()
</script>

<template>
  <div>
    <div class="flex items-center gap-2">
      <UiSwitch
        v-if="param.flag"
        class="flex-1"
        trailing
        :label="param.label"
        :disabled="busy"
        :model-value="on"
        @update:model-value="(v: boolean) => emit('set', v ? 'true' : 'false')"
      />
      <template v-else>
        <label :for="id" class="min-w-0 flex-1">{{ param.label }}</label>
        <UiSelect
          v-if="pickable"
          class="w-40 shrink-0"
          hide-label
          :label="param.label"
          :options="options"
          :model-value="current"
          @update:model-value="(v: string | null) => v !== null && emit('set', v)"
        />
        <span v-else class="flex shrink-0 items-center gap-1.5">
          <UiField
            :id="id"
            class="t-input w-24 text-right font-mono text-xs select-text"
            :value="param.value"
            :placeholder="param.default"
            :title="limits"
            :disabled="busy"
            @change="emit('set', ($event.target as HTMLInputElement).value)"
          />
          <span class="w-9 text-xs text-muted">{{ param.unit }}</span>
        </span>
      </template>
      <button
        class="t-icon-btn size-6 shrink-0"
        :class="param.value ? '' : 'invisible'"
        :disabled="!param.value || busy"
        :title="`${param.label}: back to how it comes`"
        :aria-label="`Put ${param.label} back to how it comes`"
        @click="emit('set', '')"
      >
        <IconReset class="size-4" />
      </button>
    </div>
    <p v-if="param.description" class="m-0 mt-1 text-xs text-muted">{{ param.description }}<template v-if="limits && !pickable"> From {{ limits }}.</template></p>
  </div>
</template>
