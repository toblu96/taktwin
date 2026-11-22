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
  background: var(--t-s1);
}

/* Below the 3D view the dock is a strip: each group's controls in a row under its title, the
   groups side by side as far as they fit, and on a further line when they do not. */
.bottom {
  border-top: 1px solid var(--t-line);
  display: flex;
  flex-wrap: wrap;
  gap: 6px 28px;
  padding: 6px 12px 8px;
}

.bottom .cell {
  flex: 0 1 auto;
  min-width: 0;
}

.bottom :deep(.widgets) {
  align-items: center;
  flex-flow: row wrap;
  gap: 6px;
}

.bottom :deep(.group .section-title) {
  margin-bottom: 4px;
}

.bottom :deep(.fill) {
  align-items: center;
  display: flex;
  gap: 6px;
}

.bottom :deep(.fill .note) {
  margin: 0;
}

.side {
  border-bottom: 1px solid var(--t-line);
}

.side .cell {
  padding: 12px;
}
</style>
