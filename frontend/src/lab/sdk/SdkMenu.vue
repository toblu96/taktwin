<script setup lang="ts">
import type { Component } from 'vue'
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuShortcut, MenuTrigger } from '@ui-sdk/vue/menu'
import IconMore from '~icons/fluent/more-horizontal-20-regular'

// A menu from a button, such as a project card's ⋯. A separator is an item without a label.
export type MenuEntry = { label?: string; icon?: Component; shortcut?: string; danger?: boolean; run?: () => void }
defineProps<{ items: MenuEntry[]; label: string }>()
</script>

<template>
  <Menu>
    <MenuTrigger :aria-label="label" class="t-icon-btn data-open:bg-press data-open:text-fg">
      <slot><IconMore class="size-4.5" /></slot>
    </MenuTrigger>
    <MenuContent class="t-popup">
      <template v-for="(item, i) in items" :key="i">
        <MenuSeparator v-if="!item.label" class="mx-1 my-1 h-px bg-line" />
        <MenuItem v-else class="t-item data-highlighted:bg-select" :class="item.danger ? 'text-danger' : ''" @select="item.run?.()">
          <component :is="item.icon" v-if="item.icon" class="size-4 text-muted" :class="item.danger ? 'text-danger' : ''" />
          <span class="flex-1">{{ item.label }}</span>
          <MenuShortcut v-if="item.shortcut" class="text-xs text-faint">{{ item.shortcut }}</MenuShortcut>
        </MenuItem>
      </template>
    </MenuContent>
  </Menu>
</template>
