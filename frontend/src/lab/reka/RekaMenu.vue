<script setup lang="ts">
import { DropdownMenuContent, DropdownMenuItem, DropdownMenuPortal, DropdownMenuRoot, DropdownMenuSeparator, DropdownMenuTrigger } from 'reka-ui'
import type { MenuEntry } from '../sdk/SdkMenu.vue'
import IconMore from '~icons/fluent/more-horizontal-20-regular'

// The same menu on Reka UI's dropdown menu.
defineProps<{ items: MenuEntry[]; label: string }>()
</script>

<template>
  <DropdownMenuRoot>
    <DropdownMenuTrigger :aria-label="label" class="t-icon-btn data-[state=open]:bg-press data-[state=open]:text-fg">
      <slot><IconMore class="size-4.5" /></slot>
    </DropdownMenuTrigger>
    <DropdownMenuPortal>
      <DropdownMenuContent :side-offset="4" align="end" class="t-popup">
        <template v-for="(item, i) in items" :key="i">
          <DropdownMenuSeparator v-if="!item.label" class="mx-1 my-1 h-px bg-line" />
          <DropdownMenuItem v-else class="t-item data-highlighted:bg-select" :class="item.danger ? 'text-danger' : ''" @select="item.run?.()">
            <component :is="item.icon" v-if="item.icon" class="size-4 text-muted" :class="item.danger ? 'text-danger' : ''" />
            <span class="flex-1">{{ item.label }}</span>
            <span v-if="item.shortcut" class="text-xs text-faint">{{ item.shortcut }}</span>
          </DropdownMenuItem>
        </template>
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenuRoot>
</template>
