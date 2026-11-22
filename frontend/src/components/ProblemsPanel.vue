<script setup lang="ts">
import type { SceneCheck } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import { useSim } from '../sim'
import IconWarning from '~icons/fluent/warning-16-regular'

// The problems tab of the panel: what is wrong with how the station is built, such as a sensor
// that reaches no PLC input. Clicking one shows what it is about in the 3D view, a second click
// hides it.
defineProps<{ checks: SceneCheck[] }>()
const { highlight } = useSim()

const showing = (check: SceneCheck) =>
  highlight.value?.instances.join() === (check.instances ?? []).join() && JSON.stringify(highlight.value?.at ?? null) === JSON.stringify(check.at ?? null)
function show(check: SceneCheck) {
  highlight.value = showing(check) ? null : { instances: check.instances ?? [], at: check.at ?? null }
}
</script>

<template>
  <ul v-if="checks.length" class="m-0 flex list-none flex-col p-0">
    <li v-for="(check, i) in checks" :key="i">
      <button
        class="flex w-full cursor-default items-start gap-2 rounded-[5px] border-0 px-2 py-1 text-left text-fg focus-visible:-outline-offset-2"
        :class="showing(check) ? 'bg-highlight' : 'bg-transparent hover:bg-hover'"
        :aria-pressed="showing(check)"
        title="Show it in the 3D view"
        @click="show(check)"
      >
        <IconWarning class="mt-0.5 size-4 shrink-0 text-danger" />
        {{ check.text }}
      </button>
    </li>
  </ul>
  <p v-else class="m-0 text-muted">Nothing found: the station is built so that it can work.</p>
</template>
