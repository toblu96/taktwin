<script setup lang="ts">
// The status bar: quiet, as VS Code's is now, with what is going on and nothing to read twice.
</script>

<template>
  <footer class="flex h-6 shrink-0 items-center gap-1 border-t border-line bg-s1 px-2 text-xs text-muted">
    <span class="flex items-center gap-1.5 px-1.5"><img src="/logo.svg" alt="" class="size-3.5" />Taktwin 0.3.0</span>
    <slot />
  </footer>
</template>
