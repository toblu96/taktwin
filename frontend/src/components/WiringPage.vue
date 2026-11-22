<script setup lang="ts">
import IoPanel from './IoPanel.vue'

// The wiring view of an open project, as it runs: how the station's signals reach its PLC, with
// what each is for and where it sits, and the I/O test to try them. Changing the wiring is for
// editing, in expert mode.
</script>

<template>
  <main class="min-h-0 overflow-auto bg-s0">
    <div class="mx-auto max-w-6xl px-8 py-6">
      <h2 class="m-0 text-[15px] font-semibold">Wiring</h2>
      <p class="m-0 mt-1 mb-5 text-muted">The signals between the station and its PLC: what each one is for, its variable and its address.</p>
      <IoPanel />
    </div>
  </main>
</template>
