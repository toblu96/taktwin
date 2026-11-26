<script setup lang="ts">
import { Dialog, DialogBackdrop, DialogClose, DialogContent, DialogDescription, DialogTitle } from '@ui-sdk/vue/dialog'
import IconDismiss from '~icons/fluent/dismiss-20-regular'

// A dialog over the page, with UI SDK's dialog: a title, what it is about, its content, and its
// buttons at the foot. Esc closes it, and focus goes back to what opened it. A large one fills
// the window but for a margin, for a tool of its own such as a PLC's bindings, whose content
// lays itself out edge to edge and has its own buttons. Given open without updating it, closing
// only asks, as an editor with unsaved changes does.
defineProps<{ title: string; description?: string; large?: boolean }>()
const open = defineModel<boolean>('open', { required: true })
</script>

<template>
  <Dialog v-model:open="open">
    <DialogBackdrop class="fixed inset-0 z-40 bg-black/45 transition-opacity duration-150 data-ending-style:opacity-0 data-starting-style:opacity-0" />
    <DialogContent
      class="fixed top-1/2 left-1/2 z-50 flex -translate-x-1/2 -translate-y-1/2 flex-col rounded-dialog border border-line bg-s2 shadow-pop transition-[opacity,scale] duration-150 data-ending-style:scale-[.98] data-ending-style:opacity-0 data-starting-style:scale-[.98] data-starting-style:opacity-0"
      :class="large ? 'h-[calc(100vh-64px)] w-[calc(100vw-64px)] overflow-hidden' : 'max-h-[85vh] w-[min(560px,calc(100vw-48px))]'"
    >
      <header class="flex items-start gap-3 px-5 pt-4 pb-2" :class="{ 'border-b border-line pb-3': large }">
        <div class="flex-1">
          <DialogTitle class="m-0 text-[15px] font-semibold">{{ title }}</DialogTitle>
          <DialogDescription v-if="description" class="m-0 mt-0.5 text-muted">{{ description }}</DialogDescription>
        </div>
        <DialogClose class="t-icon-btn -mr-2" aria-label="Close"><IconDismiss class="size-4.5" /></DialogClose>
      </header>
      <div v-if="large" class="flex min-h-0 flex-1 flex-col"><slot /></div>
      <div v-else class="min-h-0 overflow-auto px-5 py-3"><slot /></div>
      <footer v-if="$slots.footer" class="flex justify-end gap-2 border-t border-line px-5 py-3"><slot name="footer" /></footer>
    </DialogContent>
  </Dialog>
</template>
