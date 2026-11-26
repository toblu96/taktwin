<script setup lang="ts">
import { computed, ref } from 'vue'
import { Service } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import { edits } from '../builder'
import { useRoute } from 'vue-router'
import { openSettings } from '../router'
import { project, saveState } from '../sim'
import { update, updateAvailable } from '../update'
import IconSaved from '~icons/fluent/checkmark-16-regular'
import IconSaving from '~icons/fluent/arrow-sync-16-regular'
import IconLocked from '~icons/fluent/lock-closed-16-regular'
import IconUnsaved from '~icons/fluent/warning-16-regular'
import IconPanel from '~icons/fluent/panel-bottom-20-regular'
import { panelHere, panelOpen, togglePanel } from '../panel'

// The status bar, quiet as VS Code's is now: the app and its version on the left, then whether
// the open project's station is being built, and on the right where the project is kept. Each
// item shows a state, and a click goes to where it is changed. How the PLCs run is in the
// simulation's side bar, the I/O test in the top bar.
const route = useRoute()
const building = computed(() => !!project.value && !!route.meta.edit)

const problem = ref('')
async function reveal() {
  problem.value = ''
  try {
    if (project.value) await Service.RevealProject(project.value.path)
  } catch (e) {
    problem.value = (e as Error).message ?? String(e)
  }
}

// How the project's file stands, as the item on the right says it.
const saving = computed(() => {
  const st = saveState.value
  const where = project.value?.path ?? ''
  switch (st?.state) {
    case 'pending':
      return { icon: IconSaving, text: 'Saving', title: `Saved into ${where} in a moment: click to show the file`, warn: false }
    case 'readOnly':
      return { icon: IconLocked, text: st.unsaved ? 'Read only · not saved' : 'Read only', title: `${where} cannot be written: save a copy to keep changes`, warn: !!st.unsaved }
    case 'conflict':
      return { icon: IconUnsaved, text: 'Not saved', title: `${st.problem ?? ''} Click to show the file.`, warn: true }
    case 'failed':
      return { icon: IconUnsaved, text: 'Not saved', title: `${where} could not be written, and is tried again: ${st.problem ?? ''}`, warn: true }
  }
  return { icon: IconSaved, text: 'Saved', title: `Saved into ${where} as it changes: click to show the file`, warn: false }
})

const base = 'flex h-6 shrink-0 cursor-default items-center gap-1.5 rounded-sm border-0 px-1.5 text-xs'
const item = `${base} bg-transparent text-muted`
const button = `${item} hover:bg-hover hover:text-fg focus-visible:-outline-offset-2`
</script>

<template>
  <footer class="flex h-6 min-w-0 shrink-0 items-center gap-0.5 overflow-hidden border-t border-line bg-s1 px-1.5 whitespace-nowrap">
    <button :class="button" :title="updateAvailable ? 'An update is available' : 'Updates'" @click="openSettings('updates')">
      <img src="/logo.svg" alt="" class="size-3.5" />
      Taktwin {{ update?.version ?? '' }}
      <span v-if="updateAvailable" class="size-1.5 rounded-full bg-accent" />
    </button>

    <template v-if="building">
      <span class="mx-1 h-3 w-px bg-line" />
      <span :class="item">Building{{ edits ? ` · ${edits} ${edits === 1 ? 'change' : 'changes'}` : '' }}</span>
    </template>

    <span class="flex-1" />
    <span v-if="problem" :class="item" class="text-danger">{{ problem }}</span>
    <button v-if="panelHere" :class="button" :aria-pressed="panelOpen" title="Show or hide the panel (⌘J or Ctrl+J)" @click="togglePanel">
      <IconPanel class="size-3.5" />Panel
    </button>
    <button v-if="project" :class="[button, saving.warn ? 'text-danger hover:text-danger' : '']" :title="saving.title" @click="reveal">
      <component :is="saving.icon" class="size-3.5" />{{ saving.text }}
    </button>
  </footer>
</template>
