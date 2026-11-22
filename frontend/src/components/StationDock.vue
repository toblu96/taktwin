<script setup lang="ts">
import { computed } from 'vue'
import { useSim } from '../sim'
import WidgetList from './WidgetList.vue'

// The station's own controls, such as putting workpieces in: a strip below the 3D view, as in
// the library's preview, or, without a place, all of them one group under the other, as in the
// side bar of a running station.
const props = defineProps<{ place?: 'bottom' }>()

const { scene } = useSim()

const widgets = computed(() => (scene.value?.docks ?? []).filter((d) => !props.place || d.place === props.place).flatMap((d) => d.widgets ?? []))
</script>

<template>
  <div v-if="widgets.length && !place" class="flex flex-col gap-4">
    <WidgetList v-for="(widget, index) in widgets" :key="index" :widgets="[widget]" inline />
  </div>
  <section v-else-if="widgets.length" class="flex flex-wrap gap-x-7 gap-y-1.5 border-0 border-t border-solid border-line bg-s1 px-3 pt-1.5 pb-2">
    <div v-for="(widget, index) in widgets" :key="index" class="min-w-0">
      <WidgetList :widgets="[widget]" inline />
    </div>
  </section>
</template>
