<script setup lang="ts">
import { DialogClose, DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import IconDismiss from '~icons/fluent/dismiss-20-regular'

// The same dialog on Reka UI.
defineProps<{ title: string; description?: string }>()
const open = defineModel<boolean>('open', { required: true })
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-40 bg-black/45" />
      <DialogContent
        class="fixed top-1/2 left-1/2 z-50 flex max-h-[85vh] w-[min(560px,calc(100vw-48px))] -translate-x-1/2 -translate-y-1/2 flex-col rounded-dialog border border-line bg-s2 shadow-pop"
      >
        <header class="flex items-start gap-3 px-5 pt-4 pb-2">
          <div class="flex-1">
            <DialogTitle class="text-[15px] font-semibold">{{ title }}</DialogTitle>
            <DialogDescription v-if="description" class="mt-0.5 text-muted">{{ description }}</DialogDescription>
          </div>
          <DialogClose class="t-icon-btn -mr-2" aria-label="Close"><IconDismiss class="size-4.5" /></DialogClose>
        </header>
        <div class="min-h-0 overflow-auto px-5 py-3"><slot /></div>
        <footer class="flex justify-end gap-2 border-t border-line px-5 py-3"><slot name="footer" /></footer>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
