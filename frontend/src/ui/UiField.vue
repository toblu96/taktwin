<script setup lang="ts">
import { ref, watch } from 'vue'

// A text or number field whose value comes from outside, such as a connection's form or a live
// setting, for pages that redraw many times a second while the station runs. Vue writes a bound
// value back into its field on every redraw, which threw away what was being typed; this field
// keeps what is typed while it has focus, and shows the value from outside again once it is left.
// Everything else, such as its class, type, limits and change handler, goes to the input itself.
defineOptions({ inheritAttrs: false })
const props = defineProps<{ value?: string | number | null }>()

const shown = (v: string | number | null | undefined) => (v === null || v === undefined ? '' : String(v))
const draft = ref(shown(props.value))
const focused = ref(false)
watch(
  () => props.value,
  (v) => {
    if (!focused.value) draft.value = shown(v)
  },
)
function leave() {
  focused.value = false
  draft.value = shown(props.value)
}
</script>

<template>
  <input v-bind="$attrs" :value="draft" @input="draft = ($event.target as HTMLInputElement).value" @focus="focused = true" @blur="leave" />
</template>
