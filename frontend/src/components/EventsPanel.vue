<script setup lang="ts">
import { clearEvents, events } from '../events'
import IconWarning from '~icons/fluent/warning-16-regular'
import IconRestart from '~icons/fluent/arrow-counterclockwise-16-regular'

// The events tab of the panel: what happened while the station ran, newest last, with the
// simulated time.
const clock = (t: number) => `${String(Math.floor(t / 60)).padStart(2, '0')}:${(t % 60).toFixed(1).padStart(4, '0')}`
</script>

<template>
  <div v-if="events.length" class="flex flex-col gap-2">
    <ol class="m-0 flex list-none flex-col p-0">
      <li v-for="(e, i) in events" :key="i" class="flex items-start gap-2.5 border-0 border-b border-solid border-line py-1 last:border-b-0">
        <span class="w-14 shrink-0 font-mono text-xs leading-5 text-muted">{{ clock(e.time) }}</span>
        <component :is="e.kind === 'restart' ? IconRestart : IconWarning" class="mt-0.5 size-4 shrink-0" :class="e.kind === 'restart' ? 'text-muted' : 'text-danger'" />
        <span :class="e.kind === 'restart' ? 'text-muted' : ''">{{ e.text }}</span>
      </li>
    </ol>
    <button class="t-btn self-start" @click="clearEvents">Clear</button>
  </div>
  <p v-else class="m-0 text-muted">Nothing has happened yet: collisions and workpieces that fall off show here.</p>
</template>
