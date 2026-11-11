<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import type { Completion, Item } from './complete'
import { highlight, highlightMonitored, indentAfter, wordAt, type Monitored } from './st'

// A code editor for Structured Text: a highlighted copy of the text under a transparent
// text area that takes the typing, with line numbers and marks for messages.
//
// Online it shows the program as CODESYS does: the value of each variable in a box after its
// name, which moves the text along, so that view takes no typing. Typing or a double click
// there edits at that place, and Esc goes back to the values. A click on a value prepares
// one to write or force.
const props = defineProps<{
  modelValue: string
  readOnly?: boolean
  marks?: { line: number; severity: string; msg: string }[]
  // monitor finds a variable by its name as the code writes it; set while online.
  monitor?: (name: string) => Monitored | null
  prepared?: Record<string, string>
  // scope says whose values show, such as PLC_PRG or PLC_PRG.stepHandler.
  scope?: string
  // downloads changes with each download, which goes back to the values.
  downloads?: number
  // goto moves the cursor to a line; n changes for each jump, also to the same line.
  goto?: { line: number; col: number; n: number } | null
  // complete lists what fits at an offset of the text, for Ctrl+Space.
  complete?: (text: string, offset: number) => Completion | null
}>()
const emit = defineEmits<{ 'update:modelValue': [string]; save: []; prepare: [path: string, value: string | null] }>()

const LINE = 19 // px, as the CSS
const PAD = 8
const TAB = 4

const area = ref<HTMLTextAreaElement | null>(null)
const layer = ref<HTMLDivElement | null>(null)
const gutter = ref<HTMLDivElement | null>(null)
const mon = ref<HTMLDivElement | null>(null)
const code = ref<HTMLDivElement | null>(null)

const editing = ref(false)
const monitoring = computed(() => !!props.monitor && !editing.value)
const html = computed(() => (monitoring.value ? '' : highlight(props.modelValue)))
const monHtml = computed(() => (monitoring.value ? highlightMonitored(props.modelValue, props.monitor!, props.prepared ?? {}) : ''))

const lines = computed(() => props.modelValue.split('\n').length)
const marksByLine = computed(() => {
  const m = new Map<number, { severity: string; msg: string }[]>()
  for (const mark of props.marks ?? []) m.set(mark.line, [...(m.get(mark.line) ?? []), mark])
  return m
})
const markClass = (line: number) => {
  const list = marksByLine.value.get(line)
  if (!list) return ''
  return list.some((m) => m.severity === 'error') ? 'error' : 'warning'
}

function sync() {
  const a = area.value
  if (!a) return
  if (layer.value) layer.value.style.transform = `translate(${-a.scrollLeft}px, ${-a.scrollTop}px)`
  if (gutter.value) gutter.value.scrollTop = a.scrollTop
}

function syncMon() {
  if (gutter.value && mon.value) gutter.value.scrollTop = mon.value.scrollTop
}

function onInput(e: Event) {
  const a = e.target as HTMLTextAreaElement
  emit('update:modelValue', a.value)
  const at = a.selectionStart
  // As CODESYS: a dot after a name lists its members; typing on narrows the list.
  if (ac.value || (a.value[at - 1] === '.' && /[A-Za-z0-9_\]]/.test(a.value[at - 2] ?? ''))) void nextTick(() => suggest(false))
}

// --- Completion, as CODESYS lists components.
const ac = ref<{ from: number; items: Item[]; sel: number; x: number; y: number; above: boolean } | null>(null)
const acList = ref<HTMLUListElement | null>(null)
const KIND_LABEL: Record<string, string> = {
  local: 'VAR',
  member: 'VAR',
  io: 'I/O',
  global: 'GVL',
  program: 'PRG',
  fb: 'FB',
  function: 'FUN',
  type: 'TYPE',
  value: 'ENUM',
  keyword: 'KW',
}

// suggest opens or updates the list at the cursor. Asked for with Ctrl+Space, a single
// match is put in at once.
function suggest(asked: boolean) {
  const a = area.value
  if (!a || !props.complete || props.readOnly) return
  const at = a.selectionStart
  const c = a.selectionEnd === at ? props.complete(a.value, at) : null
  if (!c || !c.items.length) {
    ac.value = null
    return
  }
  const typed = a.value.slice(c.from, at)
  if (asked && typed && c.items.length === 1) {
    ac.value = { ...c, sel: 0, x: 0, y: 0, above: false }
    accept(0)
    return
  }
  // Where the cursor is on screen, with tabs as wide as they show.
  const lineStart = a.value.lastIndexOf('\n', c.from - 1) + 1
  const line = a.value.slice(0, c.from).split('\n').length - 1
  let col = 0
  for (const ch of a.value.slice(lineStart, c.from)) col += ch === '\t' ? TAB - (col % TAB) : 1
  const x = PAD + col * charWidth - a.scrollLeft
  const top = PAD + line * LINE - a.scrollTop
  const room = (code.value?.clientHeight ?? 400) - top - LINE
  const above = room < 220 && top > room
  const keep = ac.value && ac.value.from === c.from ? Math.min(ac.value.sel, c.items.length - 1) : 0
  ac.value = { ...c, sel: keep, x: Math.max(0, x - 4), y: above ? top : top + LINE, above }
}

function accept(i: number) {
  const a = area.value
  const c = ac.value
  ac.value = null
  if (!a || !c) return
  const item = c.items[i]
  let end = a.selectionStart
  // The rest of the word at the cursor goes as well.
  while (/[A-Za-z0-9_]/.test(a.value[end] ?? '')) end++
  a.setSelectionRange(c.from, end)
  insert(item.label)
}

function moveSel(by: number) {
  const c = ac.value!
  c.sel = (c.sel + by + c.items.length) % c.items.length
  void nextTick(() => acList.value?.children[c.sel]?.scrollIntoView({ block: 'nearest' }))
}

// onListKey handles the keys of an open list; it tells whether it took the key.
function onListKey(e: KeyboardEvent): boolean {
  if (!ac.value) return false
  const keys: Record<string, () => void> = {
    ArrowDown: () => moveSel(1),
    ArrowUp: () => moveSel(-1),
    PageDown: () => moveSel(Math.min(8, ac.value!.items.length - 1 - ac.value!.sel)),
    PageUp: () => moveSel(-Math.min(8, ac.value!.sel)),
    Enter: () => accept(ac.value!.sel),
    Tab: () => accept(ac.value!.sel),
    Escape: () => (ac.value = null),
  }
  const k = keys[e.key]
  if (!k || e.shiftKey || e.altKey || e.metaKey || e.ctrlKey) {
    if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) ac.value = null
    return false
  }
  e.preventDefault()
  e.stopPropagation()
  k()
  return true
}

// Ctrl+Space as in CODESYS; Option+Space as well, as macOS may keep Ctrl+Space for itself.
const isSuggestKey = (e: KeyboardEvent) => e.code === 'Space' && (e.ctrlKey || e.altKey) && !e.metaKey

// insert types text at the cursor, so that undo works as for typing.
function insert(text: string) {
  const a = area.value!
  if (!document.execCommand('insertText', false, text)) {
    const { selectionStart: s, selectionEnd: e } = a
    a.setRangeText(text, s, e, 'end')
    emit('update:modelValue', a.value)
  }
}

function newline() {
  const a = area.value!
  const { selectionStart: s, value } = a
  const lineStart = value.lastIndexOf('\n', s - 1) + 1
  insert('\n' + indentAfter(value.slice(lineStart, s)))
}

function onKey(e: KeyboardEvent) {
  const a = area.value!
  if (onListKey(e)) return
  if (isSuggestKey(e)) {
    e.preventDefault()
    suggest(true)
    return
  }
  if ((e.metaKey || e.ctrlKey) && e.key === 's') {
    e.preventDefault()
    emit('save')
    return
  }
  if (e.key === 'Escape' && props.monitor) {
    e.preventDefault()
    void showValues()
    return
  }
  if (props.readOnly) return
  if (e.key === 'Tab') {
    e.preventDefault()
    const { selectionStart: s, selectionEnd: end, value } = a
    const multi = value.slice(s, end).includes('\n')
    if (!multi && !e.shiftKey) {
      insert('\t')
      return
    }
    // Indent or outdent the lines of the selection.
    const from = value.lastIndexOf('\n', s - 1) + 1
    const block = value.slice(from, end)
    const changed = block
      .split('\n')
      .map((l) => (e.shiftKey ? l.replace(/^(\t| {1,4})/, '') : '\t' + l))
      .join('\n')
    a.setSelectionRange(from, end)
    insert(changed)
    a.setSelectionRange(from, from + changed.length)
    return
  }
  if (e.key === 'Enter' && !e.metaKey && !e.ctrlKey) {
    e.preventDefault()
    newline()
  }
}

// --- Online: the values in the code.

// offsetOf is the place in the source of a place in the monitoring view: the boxes have no
// text of their own, so the text before it is the source's.
function offsetOf(node: Node, offset: number): number | null {
  const pre = mon.value?.querySelector('pre')
  if (!pre || !pre.contains(node)) return null
  const r = document.createRange()
  r.setStart(pre, 0)
  r.setEnd(node, offset)
  return Math.min(r.toString().length, props.modelValue.length)
}

function offsetAtPoint(x: number, y: number): number | null {
  const r = document.caretRangeFromPoint?.(x, y)
  return r ? offsetOf(r.startContainer, r.startOffset) : null
}

// edit leaves the values for typing, with the cursor at a place of the source.
async function edit(offset: number | null) {
  if (props.readOnly) return
  const top = mon.value?.scrollTop ?? 0
  editing.value = true
  await nextTick()
  const a = area.value
  if (!a) return
  a.focus({ preventScroll: true })
  if (offset !== null) a.setSelectionRange(offset, offset)
  a.scrollTop = top
  sync()
}

// showValues goes back from typing to the values.
async function showValues() {
  const top = area.value?.scrollTop ?? 0
  editing.value = false
  hover.value = null
  await nextTick()
  if (mon.value) {
    mon.value.scrollTop = top
    mon.value.focus({ preventScroll: true })
    syncMon()
  }
}

watch(
  () => props.downloads,
  () => void showValues(),
)

function onMonKey(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key === 's') {
    e.preventDefault()
    emit('save')
    return
  }
  if (isSuggestKey(e) && !props.readOnly) {
    e.preventDefault()
    const sel = window.getSelection()
    const at = sel?.focusNode ? offsetOf(sel.focusNode, sel.focusOffset) : null
    void edit(at).then(() => suggest(true))
    return
  }
  if (props.readOnly || e.metaKey || e.ctrlKey || e.altKey) return
  const typed = e.key.length === 1
  if (!typed && !['Enter', 'Backspace', 'Delete', 'Tab'].includes(e.key)) return
  e.preventDefault()
  const sel = window.getSelection()
  const at = sel?.focusNode ? offsetOf(sel.focusNode, sel.focusOffset) : null
  void edit(at).then(() => {
    if (!area.value) return
    if (typed) insert(e.key)
    else if (e.key === 'Enter') newline()
    else if (e.key === 'Tab') insert('\t')
    else document.execCommand(e.key === 'Backspace' ? 'delete' : 'forwardDelete')
  })
}

// The value being prepared in a box, for a variable that is not a BOOL.
const prep = ref<{ path: string; x: number; y: number; text: string } | null>(null)
const prepInput = ref<HTMLInputElement | null>(null)

function onMonClick(e: MouseEvent) {
  const el = (e.target as HTMLElement).closest<HTMLElement>('.st-mon')
  // The second click of a double click is the same wish.
  if (!el || e.detail > 1 || !el.classList.contains('writable')) return
  const path = el.dataset.path!
  const value = el.dataset.value!
  const now = props.prepared?.[path]
  if (el.dataset.type === 'BOOL') {
    // As CODESYS: the other value, then this one, then none.
    const other = value === 'TRUE' ? 'FALSE' : 'TRUE'
    emit('prepare', path, now === undefined ? other : now === other ? value : null)
    return
  }
  const r = el.getBoundingClientRect()
  const c = code.value!.getBoundingClientRect()
  prep.value = { path, x: r.left - c.left, y: r.bottom - c.top + 2, text: now ?? value }
  void nextTick(() => {
    prepInput.value?.focus()
    prepInput.value?.select()
  })
}

function onMonDouble(e: MouseEvent) {
  if ((e.target as HTMLElement).closest('.st-mon')) return
  void edit(offsetAtPoint(e.clientX, e.clientY))
}

function prepareDone(keep: boolean) {
  const p = prep.value
  prep.value = null
  if (keep && p) emit('prepare', p.path, p.text.trim() === '' ? null : p.text.trim())
  mon.value?.focus({ preventScroll: true })
}

// --- Typing online: the value under the pointer, for a moment.
const hover = ref<{ x: number; y: number; text: string } | null>(null)
let charWidth = 0

function measure() {
  const probe = document.createElement('span')
  probe.className = 'st-measure'
  probe.textContent = 'MMMMMMMMMM'
  code.value?.appendChild(probe)
  charWidth = probe.getBoundingClientRect().width / 10
  probe.remove()
}

function onMove(e: MouseEvent) {
  if (!props.monitor || !area.value || !charWidth) {
    hover.value = null
    return
  }
  const a = area.value
  const r = a.getBoundingClientRect()
  const line = Math.floor((e.clientY - r.top + a.scrollTop - PAD) / LINE)
  const visual = Math.floor((e.clientX - r.left + a.scrollLeft - PAD) / charWidth)
  const all = props.modelValue.split('\n')
  if (line < 0 || line >= all.length) {
    hover.value = null
    return
  }
  // From the column on screen to the character, with tabs as wide as they show.
  const text = all[line]
  let col = 0
  let x = 0
  while (col < text.length && x + (text[col] === '\t' ? TAB - (x % TAB) : 1) <= visual) {
    x += text[col] === '\t' ? TAB - (x % TAB) : 1
    col++
  }
  const offset = all.slice(0, line).reduce((n, l) => n + l.length + 1, 0) + col
  const word = wordAt(props.modelValue, offset)
  const m = word ? props.monitor(word) : null
  hover.value = !m || !word ? null : { x: e.clientX - r.left + 12, y: e.clientY - r.top + 14, text: `${word} = ${m.value}` }
}

watch(
  () => props.goto?.n,
  async () => {
    const g = props.goto
    if (!g) return
    editing.value = true
    await nextTick()
    const a = area.value
    if (!a) return
    const all = props.modelValue.split('\n')
    const offset = all.slice(0, g.line - 1).reduce((n, l) => n + l.length + 1, 0) + Math.max(0, g.col - 1)
    a.focus()
    a.setSelectionRange(offset, offset)
    a.scrollTop = Math.max(0, (g.line - 6) * LINE)
    sync()
  },
)

function onAreaScroll() {
  ac.value = null
  sync()
}

watch(monitoring, async (on) => {
  if (!on) return
  await nextTick()
  syncMon()
})

onMounted(() => {
  measure()
  sync()
})
</script>

<template>
  <div class="editor">
    <div ref="gutter" class="gutter" aria-hidden="true">
      <div class="numbers">
        <div
          v-for="n in lines"
          :key="n"
          class="number"
          :class="markClass(n)"
          :title="
            marksByLine
              .get(n)
              ?.map((m) => m.msg)
              .join('\n')
          "
        >
          {{ n }}
        </div>
      </div>
    </div>
    <div ref="code" class="code" @mouseleave="hover = null">
      <div
        v-if="monitoring"
        ref="mon"
        class="mon"
        tabindex="0"
        aria-label="The program with its values"
        @scroll="syncMon"
        @keydown="onMonKey"
        @click="onMonClick"
        @dblclick="onMonDouble"
      >
        <div class="mon-inner">
          <div v-for="[line] in marksByLine" :key="line" class="line-mark" :class="markClass(line)" :style="{ top: `${PAD + (line - 1) * LINE}px` }" />
          <pre class="text" v-html="monHtml"></pre>
        </div>
      </div>
      <template v-else>
        <div ref="layer" class="layer" aria-hidden="true">
          <div v-for="[line] in marksByLine" :key="line" class="line-mark" :class="markClass(line)" :style="{ top: `${PAD + (line - 1) * LINE}px` }" />
          <pre class="text" v-html="html"></pre>
        </div>
        <textarea
          ref="area"
          class="input"
          :value="modelValue"
          :readonly="readOnly"
          wrap="off"
          spellcheck="false"
          autocapitalize="off"
          autocomplete="off"
          @input="onInput"
          @keydown="onKey"
          @scroll="onAreaScroll"
          @mousemove="onMove"
          @blur="ac = null"
          @mousedown="ac = null"
        />
      </template>
      <div v-if="ac" class="ac" :class="{ above: ac.above }" :style="{ left: `${ac.x}px`, top: `${ac.y}px` }" @mousedown.prevent>
        <ul ref="acList" role="listbox" aria-label="Suggestions">
          <li
            v-for="(it, i) in ac.items"
            :key="it.label + it.kind"
            role="option"
            :aria-selected="i === ac.sel"
            :class="{ sel: i === ac.sel }"
            @click="accept(i)"
            @mousemove="ac.sel = i"
          >
            <span class="k" :class="it.kind">{{ KIND_LABEL[it.kind] }}</span>
            <span class="mono label">{{ it.label }}</span>
            <span class="mono detail">{{ it.detail }}</span>
          </li>
        </ul>
        <p v-if="ac.items[ac.sel]?.doc" class="doc">{{ ac.items[ac.sel].doc }}</p>
      </div>
      <div v-if="prep" class="prep" :style="{ left: `${prep.x}px`, top: `${prep.y}px` }">
        <input
          ref="prepInput"
          v-model="prep.text"
          class="mono"
          aria-label="Prepared value"
          @keydown.enter.prevent="prepareDone(true)"
          @keydown.esc.prevent="prepareDone(false)"
          @blur="prepareDone(false)"
        />
      </div>
      <div v-if="monitor" class="mode">
        <template v-if="monitoring">
          <span class="dot" />Values of <span class="mono">{{ scope || 'the global variables' }}</span>
          <button v-if="!readOnly" title="Edit the code here; Esc shows the values again. Typing or a double click in the code does the same." @click="edit(null)">Edit</button>
        </template>
        <template v-else>
          Editing
          <button title="Show the values in the code (Esc)" @click="showValues">Show values</button>
        </template>
      </div>
      <div v-if="hover" class="hover mono" :style="{ left: `${hover.x}px`, top: `${hover.y}px` }">{{ hover.text }}</div>
    </div>
  </div>
</template>

<style scoped>
.editor {
  background: #0d2326;
  display: flex;
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

.gutter {
  border-right: 1px solid var(--line);
  color: var(--text-dim);
  flex: none;
  font: 13px/19px var(--mono);
  overflow: hidden;
  padding: 8px 0;
  text-align: right;
  width: 46px;
}

.number {
  height: 19px;
  padding-right: 8px;
}

.number.error {
  background: rgba(255, 122, 102, 0.25);
  color: var(--danger);
}

.number.warning {
  background: rgba(255, 192, 77, 0.18);
  color: var(--accent);
}

.code {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  position: relative;
}

.layer {
  left: 0;
  min-width: 100%;
  pointer-events: none;
  position: absolute;
  top: 0;
}

.text,
.input,
:deep(.st-measure) {
  font: 13px/19px var(--mono);
  tab-size: 4;
  white-space: pre;
}

.text {
  color: var(--text);
  margin: 0;
  padding: 8px;
}

:deep(.st-measure) {
  position: absolute;
  visibility: hidden;
}

.line-mark {
  height: 19px;
  left: 0;
  position: absolute;
  right: 0;
}

.line-mark.error {
  background: rgba(255, 122, 102, 0.14);
  border-bottom: 1px dashed var(--danger);
}

.line-mark.warning {
  background: rgba(255, 192, 77, 0.1);
}

.input {
  background: transparent;
  border: 0;
  caret-color: var(--accent);
  color: transparent;
  height: 100%;
  inset: 0;
  outline: none;
  overflow: auto;
  padding: 8px;
  position: absolute;
  resize: none;
  user-select: text;
  -webkit-user-select: text;
  width: 100%;
}

.input::selection {
  background: rgba(111, 240, 196, 0.25);
  color: transparent;
}

.mon {
  height: 100%;
  inset: 0;
  outline: none;
  overflow: auto;
  position: absolute;
  user-select: text;
  -webkit-user-select: text;
}

.mon-inner {
  min-width: 100%;
  position: relative;
  width: max-content;
}

.mon .text::selection,
.mon :deep(*::selection) {
  background: rgba(111, 240, 196, 0.25);
}

/* A value in the code, as CODESYS shows it: a box after the name. Its text is CSS, not part of the code. */
:deep(.st-mon) {
  background: #e9eef0;
  border: 1px solid #9fb3b8;
  border-radius: 2px;
  color: #10282c;
  display: inline-block;
  font-size: 11px;
  height: 15px;
  line-height: 13px;
  margin: 2px 2px 0 3px;
  padding: 0 4px;
  user-select: none;
  -webkit-user-select: none;
  vertical-align: top;
}

:deep(.st-mon::after) {
  content: attr(data-v);
}

:deep(.st-mon.writable) {
  cursor: pointer;
}

:deep(.st-mon.on) {
  background: #2f6fd6;
  border-color: #2f6fd6;
  color: #fff;
}

:deep(.st-mon.forced::before) {
  background: #d9412b;
  border-radius: 2px;
  color: #fff;
  content: 'F';
  font-weight: 700;
  margin-right: 3px;
  padding: 0 2px;
}

.ac {
  background: var(--panel-raised);
  border: 1px solid var(--line);
  border-radius: 3px;
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.4);
  max-width: 560px;
  min-width: 300px;
  position: absolute;
  z-index: 4;
}

.ac.above {
  transform: translateY(-100%);
}

.ac ul {
  list-style: none;
  margin: 0;
  max-height: 210px;
  overflow: auto;
  padding: 2px 0;
}

.ac li {
  align-items: baseline;
  cursor: pointer;
  display: flex;
  font-size: 12px;
  gap: 8px;
  padding: 2px 8px;
  white-space: nowrap;
}

.ac li.sel {
  background: rgba(127, 183, 255, 0.22);
}

.ac .k {
  color: var(--text-dim);
  flex: none;
  font-size: 9px;
  font-weight: 700;
  text-align: center;
  width: 30px;
}

.ac .k.io {
  color: #ff8fc7;
}

.ac .k.local,
.ac .k.member {
  color: #6ff0c4;
}

.ac .k.fb,
.ac .k.program,
.ac .k.function {
  color: #c9a7ff;
}

.ac .k.keyword {
  color: #7fb7ff;
}

.ac .label {
  color: var(--text);
}

.ac .detail {
  color: var(--text-dim);
  margin-left: auto;
  overflow: hidden;
  padding-left: 12px;
  text-overflow: ellipsis;
}

.ac .doc {
  border-top: 1px solid var(--line);
  color: var(--text-dim);
  font-size: 11px;
  margin: 0;
  padding: 4px 8px;
  white-space: normal;
}

.prep {
  position: absolute;
  z-index: 3;
}

.prep input {
  background: var(--bg);
  border: 1px solid var(--accent);
  border-radius: 2px;
  color: var(--text);
  font-size: 12px;
  padding: 2px 5px;
  width: 140px;
}

.mode {
  align-items: center;
  background: rgba(19, 46, 50, 0.92);
  border: 1px solid var(--line);
  border-radius: 3px;
  color: var(--text-dim);
  display: flex;
  font-size: 12px;
  gap: 6px;
  padding: 3px 4px 3px 8px;
  position: absolute;
  right: 16px;
  top: 6px;
  z-index: 2;
}

.mode .dot {
  background: var(--signal);
  border-radius: 50%;
  height: 7px;
  width: 7px;
}

.mode button {
  font-size: 11px;
  padding: 1px 7px;
}

.hover {
  background: var(--panel-raised);
  border: 1px solid var(--line);
  border-radius: 2px;
  font-size: 12px;
  padding: 2px 6px;
  pointer-events: none;
  position: absolute;
  white-space: nowrap;
  z-index: 2;
}

:deep(.st-keyword) {
  color: #7fb7ff;
  font-weight: 600;
}

:deep(.st-type) {
  color: #6ff0c4;
}

:deep(.st-function) {
  color: #c9a7ff;
}

:deep(.st-comment) {
  color: #6f8c89;
  font-style: italic;
}

:deep(.st-string) {
  color: #ffb37a;
}

:deep(.st-number),
:deep(.st-bool) {
  color: #ffd76e;
}

:deep(.st-address) {
  color: #ff8fc7;
}
</style>
