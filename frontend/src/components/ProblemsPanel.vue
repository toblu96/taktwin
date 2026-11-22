<script setup lang="ts">
import type { SceneCheck } from '../../bindings/github.com/toblu96/taktwin/internal/model'
import { simulation } from '../sim'
import IconWarning from '~icons/fluent/warning-16-regular'

// The problems tab of the panel: what is wrong with how the station is built, such as a sensor
// that sees nothing. Clicking one shows what it is about in the 3D view, a second click hides it.
const { scene, highlight } = simulation

const key = (check: SceneCheck) => (check.instances ?? []).join()
const showing = (check: SceneCheck) => highlight.value?.instances.join() === key(check)
function show(check: SceneCheck) {
  highlight.value = showing(check) ? null : { instances: check.instances ?? [], at: check.at ?? null }
}
</script>

<template>
  <ul v-if="scene?.checks?.length" class="m-0 flex list-none flex-col p-0">
    <li v-for="(check, i) in scene.checks" :key="i">
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
  <p v-else class="m-0 text-muted">Nothing is wrong with how the station is built.</p>
</template>
