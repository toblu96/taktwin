<script setup lang="ts">
import { defineAsyncComponent } from 'vue'

const ProgramIde = defineAsyncComponent(() => import('./ProgramIde.vue'))

// The programming tool of a PLC in a window of its own, beside the simulation.
defineProps<{ plc: string }>()
</script>

<template>
  <ProgramIde :plc="plc" window />
</template>
