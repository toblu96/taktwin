<script setup lang="ts">
import type { Component } from 'vue'
import IconSettings from '~icons/fluent/settings-20-regular'

// The bar along the left edge, as in VS Code: where you are, one icon each, with the active one
// marked by an amber line. Settings sit at the foot. The first item can be the way back.
defineProps<{ items: { id: string; label: string; icon: Component; back?: boolean }[] }>()
const active = defineModel<string>({ required: true })
</script>

<template>
  <nav class="flex w-12 shrink-0 flex-col items-center gap-1 border-r border-line bg-s1 py-2" aria-label="Views">
    <template v-for="item in items" :key="item.id">
      <button
        class="relative flex size-9 items-center justify-center rounded-control text-muted transition-colors hover:bg-hover hover:text-fg"
        :class="[active === item.id ? 'bg-press text-fg' : '', item.back ? 'mb-2' : '']"
        :aria-current="active === item.id ? 'page' : undefined"
        :title="item.label"
        @click="active = item.id"
      >
        <span v-if="active === item.id && !item.back" class="absolute top-2 bottom-2 -left-1.5 w-0.5 rounded-full bg-accent" />
        <component :is="item.icon" class="size-5" />
      </button>
      <span v-if="item.back" class="mb-1 h-px w-6 bg-line" />
    </template>
    <span class="flex-1" />
    <button class="flex size-9 items-center justify-center rounded-control text-muted hover:bg-hover hover:text-fg" title="Settings">
      <IconSettings class="size-5" />
    </button>
  </nav>
</template>
