<script setup lang="ts">
import { clearEvents, events } from '../events'
import IconWarning from '~icons/fluent/warning-16-regular'
import IconRestart from '~icons/fluent/arrow-counterclockwise-16-regular'
import IconConnected from '~icons/fluent/plug-connected-16-regular'
import IconDisconnected from '~icons/fluent/plug-disconnected-16-regular'
import type { SimEvent } from '../events'
import type { Component } from 'vue'

// The events tab of the panel: what happened while the station ran, newest last, with the time
// of day. Trouble is red, a connection made green, the rest quiet.
const LOOKS: Record<SimEvent['kind'], { icon: Component; tone: string; text: string }> = {
  hit: { icon: IconWarning, tone: 'text-danger', text: '' },
  fell: { icon: IconWarning, tone: 'text-danger', text: '' },
  lost: { icon: IconDisconnected, tone: 'text-danger', text: '' },
  connected: { icon: IconConnected, tone: 'text-ok', text: '' },
  disconnected: { icon: IconDisconnected, tone: 'text-muted', text: 'text-muted' },
  restart: { icon: IconRestart, tone: 'text-muted', text: 'text-muted' },
}
// The time of day, to the second, as the computer's clock has it.
const clock = (t: number) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
</script>

<template>
  <div v-if="events.length" class="flex flex-col gap-2">
    <ol class="m-0 flex list-none flex-col p-0">
      <li v-for="(e, i) in events" :key="i" class="flex items-start gap-2.5 border-0 border-b border-solid border-line py-1 last:border-b-0">
        <span class="w-14 shrink-0 font-mono text-xs leading-5 text-muted">{{ clock(e.time) }}</span>
        <component :is="LOOKS[e.kind].icon" class="mt-0.5 size-4 shrink-0" :class="LOOKS[e.kind].tone" />
        <span class="min-w-0 break-words" :class="LOOKS[e.kind].text">{{ e.text }}<span v-if="e.detail" class="block text-xs text-muted">{{ e.detail }}</span></span>
      </li>
    </ol>
    <button class="t-btn self-start" @click="clearEvents">Clear</button>
  </div>
  <p v-else class="m-0 text-muted">Nothing has happened yet: collisions, workpieces that fall off and how the PLCs connect show here.</p>
</template>
