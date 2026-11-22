<script setup lang="ts">
// A signal's lamp, as on a PLC's I/O card: green for an input, amber for an output, glowing while
// on, a dark ring while off.
defineProps<{ on: boolean; out?: boolean }>()
</script>

<template>
  <span
    class="block size-2.5 shrink-0 rounded-full"
    :class="on ? (out ? 'bg-lamp-out shadow-[0_0_6px_var(--t-lamp-out)]' : 'bg-lamp-in shadow-[0_0_6px_var(--t-lamp-in)]') : 'bg-s3 shadow-[inset_0_0_0_1px_var(--t-control)]'"
  />
</template>
