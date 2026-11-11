<script setup lang="ts">
import ProgramIde from './ProgramIde.vue'

// The programming tool of a PLC in a window of its own, beside the simulation.
defineProps<{ plc: string }>()
</script>

<template>
  <ProgramIde :plc="plc" window />
</template>
