<script setup lang="ts">
import { computed } from 'vue'
import { useSim } from '../sim'
import WidgetList from './WidgetList.vue'

const props = defineProps<{ place: 'bottom' | 'side' }>()

const { scene } = useSim()

const widgets = computed(() => (scene.value?.docks ?? []).filter((d) => d.place === props.place).flatMap((d) => d.widgets ?? []))
</script>

<template>
  <section v-if="widgets.length" class="dock" :class="place">
    <div v-for="(widget, index) in widgets" :key="index" class="cell">
      <WidgetList :widgets="[widget]" />
    </div>
  </section>
</template>

<style scoped>
.dock {
  background: var(--panel);
}

.bottom {
  border-top: 1px solid var(--line);
  display: grid;
  grid-auto-columns: minmax(150px, 1fr);
  grid-auto-flow: column;
}

.bottom .cell {
  border-right: 1px solid var(--line);
  min-width: 0;
  padding: 10px 12px;
}

.bottom .cell:last-child {
  border-right: 0;
}

.bottom .cell :deep(.group > .widgets) {
  align-items: flex-start;
  flex-flow: row wrap;
  justify-content: space-between;
}

.side {
  border-bottom: 1px solid var(--line);
}

.side .cell {
  padding: 12px;
}
</style>
