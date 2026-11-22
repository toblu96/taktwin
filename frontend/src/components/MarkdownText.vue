<script setup lang="ts">
import { computed } from 'vue'
import { markdown, type Span } from '../markdown'

// Shows Markdown text, such as an exercise's instructions, as text: nothing in it runs. It reads
// as the rest of the app does, with headings a step up, lists indented and code in the mono font
// on a darker ground, and it can be selected to copy.
const props = defineProps<{ source: string }>()
const blocks = computed(() => markdown(props.source))
const tag = (s: Span) => (s.code ? 'code' : s.bold ? 'strong' : s.em ? 'em' : 'span')
const spanClass = (s: Span) => (s.code ? 'rounded-sm bg-s3 px-1 font-mono text-xs' : s.bold ? 'font-semibold' : '')
const heading = (level: number) => (level <= 1 ? 'mt-4 mb-1.5 text-[15px] font-semibold first:mt-0' : 'mt-3 mb-1 text-[13px] font-semibold first:mt-0')
</script>

<template>
  <div class="leading-relaxed select-text">
    <template v-for="(b, i) in blocks" :key="i">
      <component :is="`h${Math.min(b.level + 3, 6)}`" v-if="b.kind === 'heading'" :class="heading(b.level)">
        <component :is="tag(s)" v-for="(s, j) in b.spans" :key="j" :class="spanClass(s)">{{ s.text }}</component>
      </component>
      <p v-else-if="b.kind === 'paragraph'" class="m-0 mb-2.5">
        <component :is="tag(s)" v-for="(s, j) in b.spans" :key="j" :class="spanClass(s)">{{ s.text }}</component>
      </p>
      <component :is="b.ordered ? 'ol' : 'ul'" v-else class="m-0 mb-2.5 flex flex-col gap-1 pl-5">
        <li v-for="(item, j) in b.items" :key="j">
          <component :is="tag(s)" v-for="(s, k) in item" :key="k" :class="spanClass(s)">{{ s.text }}</component>
        </li>
      </component>
    </template>
  </div>
</template>
