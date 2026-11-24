<script setup lang="ts">
import type { WatchVar } from '../../bindings/github.com/toblu96/taktwin/st/models'
import IconOpen from '~icons/fluent/chevron-down-12-regular'
import IconClosed from '~icons/fluent/chevron-right-12-regular'

// The online variables as a tree: each with its value, and for one that can be written, a
// prepared value to write once or force.
const props = defineProps<{ vars: WatchVar[]; depth: number; open: Set<string>; prepared: Record<string, string> }>()
const emit = defineEmits<{ toggle: [string]; write: [string]; force: [string]; release: [string] }>()

const isBool = (v: WatchVar) => v.type === 'BOOL'

// A click on a BOOL's prepared value toggles it, as in CODESYS.
function prepareBool(v: WatchVar) {
  props.prepared[v.path] = (props.prepared[v.path] ?? v.value) === 'TRUE' ? 'FALSE' : 'TRUE'
}
const none = (v: WatchVar) => props.prepared[v.path] === undefined || props.prepared[v.path] === ''
const small = 't-btn h-6 px-2 text-xs'
</script>

<template>
  <template v-for="v in vars" :key="v.path">
    <tr class="border-0 border-b border-solid border-line">
      <td class="px-1.5 py-0.5 whitespace-nowrap" :style="{ paddingLeft: `${6 + depth * 14}px` }">
        <span class="flex items-center gap-1">
          <button
            v-if="v.children?.length"
            class="t-icon-btn size-5"
            :aria-expanded="open.has(v.path)"
            :aria-label="`${open.has(v.path) ? 'Close' : 'Open'} ${v.name}`"
            @click="emit('toggle', v.path)"
          >
            <component :is="open.has(v.path) ? IconOpen : IconClosed" class="size-3" />
          </button>
          <span v-else class="inline-block w-5" />
          <span class="font-mono">{{ v.name }}</span>
        </span>
      </td>
      <td class="px-1.5 py-0.5 font-mono whitespace-nowrap text-muted">{{ v.type }}</td>
      <td class="px-1.5 py-0.5 font-mono whitespace-nowrap" :class="v.forced ? 'text-accent-ink' : v.value === 'TRUE' ? 'text-ok' : ''">
        <span v-if="v.forced" class="mr-1.5 rounded-sm bg-accent px-1 text-[10px] font-bold text-on-accent" title="Forced">F</span>{{ v.value }}
      </td>
      <td class="px-1.5 py-0.5">
        <template v-if="v.writable">
          <button v-if="isBool(v)" class="t-btn h-6 min-w-16 px-2 font-mono text-xs" :aria-label="`Prepared value of ${v.name}`" @click="prepareBool(v)">
            {{ prepared[v.path] ?? '' }}
          </button>
          <input
            v-else
            v-model="prepared[v.path]"
            class="t-input h-6 w-32 px-1.5 font-mono text-xs"
            :placeholder="v.type"
            :aria-label="`Prepared value of ${v.name}`"
            @keydown.enter="emit('write', v.path)"
          />
        </template>
      </td>
      <td class="px-1.5 py-0.5 whitespace-nowrap">
        <span v-if="v.writable" class="flex gap-1">
          <button :class="small" :disabled="none(v)" title="Write the prepared value once (Ctrl+F7)" @click="emit('write', v.path)">Write</button>
          <button :class="small" :disabled="none(v)" title="Hold the variable at the prepared value (F7)" @click="emit('force', v.path)">Force</button>
          <button v-if="v.forced" :class="small" title="Let it go (Alt+F7)" @click="emit('release', v.path)">Release</button>
        </span>
      </td>
    </tr>
    <WatchTree
      v-if="v.children?.length && open.has(v.path)"
      :vars="v.children"
      :depth="depth + 1"
      :open="open"
      :prepared="prepared"
      @toggle="emit('toggle', $event)"
      @write="emit('write', $event)"
      @force="emit('force', $event)"
      @release="emit('release', $event)"
    />
  </template>
</template>
