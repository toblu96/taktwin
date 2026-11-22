<script setup lang="ts">
import { computed } from 'vue'
import { useSim } from '../sim'
import WidgetList from './WidgetList.vue'

// The station's own controls, such as placing workpieces: a strip below the 3D view, or a part
// of the side bar.
const props = defineProps<{ place: 'bottom' | 'side' }>()

const { scene } = useSim()

const widgets = computed(() => (scene.value?.docks ?? []).filter((d) => d.place === props.place).flatMap((d) => d.widgets ?? []))
</script>

<template>
  <section
    v-if="widgets.length"
    class="bg-s1"
    :class="place === 'bottom' ? 'flex flex-wrap gap-x-7 gap-y-1.5 border-0 border-t border-solid border-line px-3 pt-1.5 pb-2' : 'border-0 border-b border-solid border-line'"
  >
    <div v-for="(widget, index) in widgets" :key="index" class="min-w-0" :class="place === 'side' ? 'p-3' : ''">
      <WidgetList :widgets="[widget]" :inline="place === 'bottom'" />
    </div>
  </section>
</template>
