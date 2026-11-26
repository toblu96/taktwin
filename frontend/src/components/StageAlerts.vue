<script setup lang="ts">
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { simulation } from '../sim'
import IconWarning from '~icons/fluent/warning-16-filled'
import IconAllOff from '~icons/fluent/circle-off-16-regular'
import IconEnd from '~icons/fluent/dismiss-16-regular'

// What happened on the station that needs an answer, over the top of the 3D view: something ran
// into something, or workpieces fell off. A fall stays until it is acknowledged. During the I/O
// test an amber frame runs round the view, so that the outputs being off the programs is never
// missed, with a bar in its corner that has what goes with the test, as a shared screen has Stop.
const { snapshot } = simulation
const onAmber =
  'inline-flex h-5.5 cursor-default items-center gap-1 rounded-[4px] border-0 bg-transparent px-1.5 text-xs font-medium text-on-accent hover:bg-on-accent/12 [--t-ring-color:var(--t-on-accent)]'
</script>

<template>
  <div v-if="snapshot?.ioTest" class="pointer-events-none absolute inset-0 z-10 border-2 border-solid border-accent">
    <div
      class="pointer-events-auto absolute bottom-0 left-0 flex items-center gap-1 rounded-tr-card bg-accent py-1 pr-1 pl-2 text-xs text-on-accent"
      role="toolbar"
      aria-label="I/O test"
    >
      <span class="mr-1 font-semibold">I/O test: outputs by hand</span>
      <span class="h-3.5 w-px bg-on-accent/30" />
      <button type="button" :class="onAmber" title="Switch every output off" @click="Service.ClearTestOutputs()"><IconAllOff class="size-3.5" />All outputs off</button>
      <button type="button" :class="onAmber" title="End the I/O test: the programs take the outputs back" @click="Service.SetIOTest(false)">
        <IconEnd class="size-3.5" />End test
      </button>
    </div>
  </div>
  <div class="pointer-events-none absolute inset-x-0 top-3 z-10 flex flex-col items-center gap-2 px-4" role="status">
    <div
      v-for="text in snapshot?.collisions ?? []"
      :key="text"
      class="pointer-events-auto flex max-w-full items-center gap-2 rounded-card border border-danger/40 bg-s2 py-1.5 pr-3 pl-3 shadow-pop"
    >
      <IconWarning class="size-4 shrink-0 text-danger" />
      <span class="truncate">{{ text }}</span>
    </div>
    <div v-if="snapshot?.fellOff" class="pointer-events-auto flex items-center gap-2 rounded-card border border-danger/40 bg-s2 py-1.5 pr-1.5 pl-3 shadow-pop" role="alert">
      <IconWarning class="size-4 shrink-0 text-danger" />
      <span>{{ snapshot.fellOff === 1 ? 'A workpiece' : `${snapshot.fellOff} workpieces` }} fell off the station.</span>
      <button class="t-btn h-6 px-2 text-xs" @click="Service.AcknowledgeFellOff()">Acknowledge</button>
    </div>
  </div>
</template>
