<script setup lang="ts">
import { computed } from 'vue'
import { markdown, type Span } from '../markdown'

// Shows Markdown text, such as an exercise's instructions, as text: nothing in it runs.
const props = defineProps<{ source: string }>()
const blocks = computed(() => markdown(props.source))
const tag = (s: Span) => (s.code ? 'code' : s.bold ? 'strong' : s.em ? 'em' : 'span')
</script>

<template>
  <div class="markdown">
    <template v-for="(b, i) in blocks" :key="i">
      <component :is="`h${Math.min(b.level + 3, 6)}`" v-if="b.kind === 'heading'">
        <component :is="tag(s)" v-for="(s, j) in b.spans" :key="j">{{ s.text }}</component>
      </component>
      <p v-else-if="b.kind === 'paragraph'">
        <component :is="tag(s)" v-for="(s, j) in b.spans" :key="j">{{ s.text }}</component>
      </p>
      <component :is="b.ordered ? 'ol' : 'ul'" v-else>
        <li v-for="(item, j) in b.items" :key="j">
          <component :is="tag(s)" v-for="(s, k) in item" :key="k">{{ s.text }}</component>
        </li>
      </component>
    </template>
  </div>
</template>

<style scoped>
.markdown {
  font-size: 13px;
  line-height: 1.5;
  user-select: text;
  -webkit-user-select: text;
}

h4,
h5,
h6 {
  font-size: 13px;
  margin: 10px 0 4px;
}

h4 {
  font-size: 14px;
}

p {
  margin: 0 0 8px;
}

ul,
ol {
  margin: 0 0 8px;
  padding-left: 18px;
}

code {
  background: var(--bg);
  border-radius: 2px;
  font-family: var(--mono);
  font-size: 12px;
  padding: 0 3px;
}
</style>
