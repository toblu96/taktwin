<script setup lang="ts">
import type { WatchVar } from '../../bindings/github.com/toblu96/taktwin/st/models'

// The online variables as a tree: each with its value, and for one that can be written, a
// prepared value to write once or force.
const props = defineProps<{ vars: WatchVar[]; depth: number; open: Set<string>; prepared: Record<string, string> }>()
const emit = defineEmits<{ toggle: [string]; write: [string]; force: [string]; release: [string] }>()

const isBool = (v: WatchVar) => v.type === 'BOOL'
// A click on a BOOL's prepared value toggles it, as in CODESYS.
function prepareBool(v: WatchVar) {
  props.prepared[v.path] = (props.prepared[v.path] ?? v.value) === 'TRUE' ? 'FALSE' : 'TRUE'
}
</script>

<template>
  <template v-for="v in vars" :key="v.path">
    <tr :class="{ forced: v.forced }">
      <td class="name" :style="{ paddingLeft: `${6 + depth * 14}px` }">
        <button v-if="v.children?.length" class="twist" :aria-expanded="open.has(v.path)" @click="emit('toggle', v.path)">{{ open.has(v.path) ? '▾' : '▸' }}</button>
        <span v-else class="twist-space" />
        <span class="mono">{{ v.name }}</span>
      </td>
      <td class="type mono">{{ v.type }}</td>
      <td class="value mono" :class="{ on: v.value === 'TRUE' }"><span v-if="v.forced" class="f" title="Forced">F</span>{{ v.value }}</td>
      <td class="prepared">
        <template v-if="v.writable">
          <button v-if="isBool(v)" class="mono prep" @click="prepareBool(v)">{{ prepared[v.path] ?? '' }}</button>
          <input v-else v-model="prepared[v.path]" class="mono prep" :placeholder="v.type" @keydown.enter="emit('write', v.path)" />
        </template>
      </td>
      <td class="actions">
        <template v-if="v.writable">
          <button :disabled="prepared[v.path] === undefined || prepared[v.path] === ''" title="Write the prepared value once (Ctrl+F7)" @click="emit('write', v.path)">
            Write
          </button>
          <button :disabled="prepared[v.path] === undefined || prepared[v.path] === ''" title="Hold the variable at the prepared value (F7)" @click="emit('force', v.path)">
            Force
          </button>
          <button v-if="v.forced" title="Let it go (Alt+F7)" @click="emit('release', v.path)">Release</button>
        </template>
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

<style scoped>
td {
  border-bottom: 1px solid rgba(35, 74, 80, 0.5);
  padding: 2px 6px;
  white-space: nowrap;
}

.name {
  display: flex;
  gap: 4px;
}

.twist {
  background: none;
  border: 0;
  color: var(--text-dim);
  padding: 0;
  width: 14px;
}

.twist-space {
  display: inline-block;
  width: 14px;
}

.type {
  color: var(--text-dim);
}

.value.on {
  color: var(--signal);
}

.forced .value {
  color: var(--accent);
}

.f {
  background: var(--accent);
  border-radius: 2px;
  color: var(--bg);
  font-size: 10px;
  font-weight: 700;
  margin-right: 5px;
  padding: 0 3px;
}

.prep {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 2px;
  min-height: 22px;
  padding: 1px 5px;
  text-align: left;
  width: 110px;
}

.actions button {
  font-size: 11px;
  margin-right: 4px;
  padding: 1px 6px;
}
</style>
