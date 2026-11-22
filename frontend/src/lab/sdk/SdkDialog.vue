<script setup lang="ts">
import { Dialog, DialogBackdrop, DialogClose, DialogContent, DialogDescription, DialogTitle } from '@ui-sdk/vue/dialog'
import IconDismiss from '~icons/fluent/dismiss-20-regular'

// A dialog over the page: a title, what it is about, its content, and its buttons at the foot.
defineProps<{ title: string; description?: string }>()
const open = defineModel<boolean>('open', { required: true })
</script>

<template>
  <Dialog v-model:open="open">
    <DialogBackdrop class="fixed inset-0 z-40 bg-black/45 transition-opacity duration-150 data-ending-style:opacity-0 data-starting-style:opacity-0" />
    <DialogContent
      class="fixed top-1/2 left-1/2 z-50 flex max-h-[85vh] w-[min(560px,calc(100vw-48px))] -translate-x-1/2 -translate-y-1/2 flex-col rounded-dialog border border-line bg-s2 shadow-pop transition-[opacity,scale] duration-150 data-ending-style:scale-[.98] data-ending-style:opacity-0 data-starting-style:scale-[.98] data-starting-style:opacity-0"
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
  </Dialog>
</template>
