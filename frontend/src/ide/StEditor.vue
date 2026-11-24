<script setup lang="ts">
import { autocompletion, closeBrackets, closeBracketsKeymap, completionKeymap, startCompletion, type CompletionContext, type CompletionResult } from '@codemirror/autocomplete'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { bracketMatching, foldGutter, foldKeymap, indentOnInput, indentUnit } from '@codemirror/language'
import { lintGutter, lintKeymap, setDiagnostics, type Action, type Diagnostic } from '@codemirror/lint'
import { highlightSelectionMatches, searchKeymap } from '@codemirror/search'
import { Compartment, EditorSelection, EditorState, Prec, StateEffect, StateField, type Range } from '@codemirror/state'
import {
  Decoration,
  drawSelection,
  EditorView,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  hoverTooltip,
  keymap,
  lineNumbers,
  rectangularSelection,
  crosshairCursor,
  dropCursor,
  WidgetType,
  type DecorationSet,
} from '@codemirror/view'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import type { Completion } from './complete'
import { tokenize, wordAt, type Monitored } from './st'
import { closeBlock, structuredText } from './stLanguage'

// A code editor for Structured Text, on CodeMirror: what VS Code does with text, such as moving
// lines (Alt+↑/↓), several cursors, search and replace, folding and comments (Ctrl/⌘+/), with
// CODESYS's completion (Ctrl+Space), Format Document (Shift+Alt+F), mistakes underlined as they
// are typed, a quick fix to declare a name, and blocks closed as they are opened.
//
// Online it shows the program as CODESYS does: the value of each variable in a box after its
// name, while the code can still be edited. A click on a value prepares one to write or force.
const props = defineProps<{
  modelValue: string
  readOnly?: boolean
  marks?: { line: number; col?: number; severity: string; msg: string }[]
  // monitor finds a variable by its name as the code writes it; set while online.
  monitor?: (name: string) => Monitored | null
  prepared?: Record<string, string>
  // scope says whose values show, such as PLC_PRG or PLC_PRG.stepHandler.
  scope?: string
  // downloads changes with each download, which shows the values again.
  downloads?: number
  // goto moves the cursor to a line; n changes for each jump, also to the same line.
  goto?: { line: number; col: number; n: number } | null
  // complete lists what fits at an offset of the text, for Ctrl+Space.
  complete?: (text: string, offset: number) => Completion | null
  // place is where the cursor and the view were left, to come back to.
  place?: { cursor: number; top: number } | null
}>()
const emit = defineEmits<{
  'update:modelValue': [string]
  save: []
  format: []
  prepare: [path: string, value: string | null]
  place: [place: { cursor: number; top: number }]
}>()

const host = ref<HTMLDivElement | null>(null)
const view = shallowRef<EditorView | null>(null)
const readOnlyConf = new Compartment()

// --- Values while online, as widgets after the names.

class ValueWidget extends WidgetType {
  constructor(
    readonly m: Monitored,
    readonly prepared: string | undefined,
  ) {
    super()
  }
  eq(o: ValueWidget) {
    return o.m.path === this.m.path && o.m.value === this.m.value && o.m.forced === this.m.forced && o.prepared === this.prepared
  }
  toDOM() {
    const el = document.createElement('span')
    const m = this.m
    el.className = ['cm-value', m.type === 'BOOL' ? (m.value === 'TRUE' ? 'on' : 'off') : '', m.forced ? 'forced' : '', m.writable ? 'writable' : ''].filter(Boolean).join(' ')
    el.textContent = this.prepared === undefined ? m.value : `${m.value} <${this.prepared}>`
    el.dataset.path = m.path
    el.dataset.type = m.type
    el.dataset.value = m.value
    el.title = m.writable ? `${m.path} : ${m.type}. Click to prepare a value, then F7 forces it, Ctrl+F7 writes it once.` : `${m.path} : ${m.type}`
    return el
  }
  ignoreEvent() {
    return false
  }
}

const setValues = StateEffect.define<Range<Decoration>[]>()
const valuesField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    for (const e of tr.effects) if (e.is(setValues)) return Decoration.set(e.value, true)
    return deco.map(tr.changes)
  },
  provide: (f) => EditorView.decorations.from(f),
})

const showValues = ref(true)
watch(
  () => props.downloads,
  () => (showValues.value = true),
)

// Where each value goes: after a name, a dotted one such as t.Q as one; the formal
// parameters of a call, IN := in t(IN := x), have none. It reads the values, so that it
// follows them as they change.
const values = computed(() => {
  if (!props.monitor || !showValues.value) return []
  const src = props.modelValue
  const toks = tokenize(src)
  const significant = (i: number, step: number) => {
    for (let j = i + step; j >= 0 && j < toks.length; j += step) if (toks[j].kind !== 'space' && toks[j].kind !== 'comment') return toks[j]
    return null
  }
  const out: { at: number; m: Monitored; prepared?: string }[] = []
  for (let i = 0; i < toks.length; i++) {
    if (toks[i].kind !== 'ident') continue
    const first = i
    while (toks[i + 1]?.text === '.' && toks[i + 2]?.kind === 'ident') i += 2
    const name = toks
      .slice(first, i + 1)
      .map((x) => x.text)
      .join('')
    const before = significant(first, -1)?.text
    const after = significant(i, 1)?.text
    if (before === '.' || after === '=>' || (after === ':=' && (before === '(' || before === ','))) continue
    const m = props.monitor(name)
    if (m) out.push({ at: toks[i].start + toks[i].text.length, m, prepared: props.prepared?.[m.path] })
  }
  return out
})

watch(values, (list) => {
  const v = view.value
  if (!v) return
  const len = v.state.doc.length
  const deco = list.filter((x) => x.at <= len).map((x) => Decoration.widget({ widget: new ValueWidget(x.m, x.prepared), side: 1 }).range(x.at))
  v.dispatch({ effects: setValues.of(deco) })
})

// The value being prepared in a box, for a variable that is not a BOOL.
const prep = ref<{ path: string; x: number; y: number; text: string } | null>(null)
const prepInput = ref<HTMLInputElement | null>(null)

function clickValue(el: HTMLElement) {
  if (!el.classList.contains('writable')) return
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
  const c = host.value!.parentElement!.getBoundingClientRect()
  prep.value = { path, x: r.left - c.left, y: r.bottom - c.top + 2, text: now ?? value }
  void nextTick(() => {
    prepInput.value?.focus()
    prepInput.value?.select()
  })
}

function prepareDone(keep: boolean) {
  const p = prep.value
  prep.value = null
  if (keep && p) emit('prepare', p.path, p.text.trim() === '' ? null : p.text.trim())
  view.value?.focus()
}

// --- Completion, as CODESYS lists components.

const KINDS = ['local', 'member', 'io', 'global', 'program', 'fb', 'function', 'type', 'value', 'keyword']

function completions(ctx: CompletionContext): CompletionResult | null {
  if (!props.complete || props.readOnly) return null
  const word = ctx.matchBefore(/[A-Za-z0-9_]*/)
  const afterDot = ctx.state.sliceDoc(ctx.pos - 1, ctx.pos) === '.'
  if (!ctx.explicit && !afterDot && (!word || word.from === word.to)) return null
  const c = props.complete(ctx.state.doc.toString(), ctx.pos)
  if (!c || !c.items.length) return null
  return {
    from: c.from,
    filter: false,
    options: c.items.map((it, i) => ({ label: it.label, detail: it.detail, info: it.doc || undefined, type: KINDS.includes(it.kind) ? it.kind : undefined, boost: -i })),
  }
}

// --- Hover: a variable's value online, or what it is.

const hover = hoverTooltip((v, pos) => {
  const src = v.state.doc.toString()
  const word = wordAt(src, pos)
  if (!word) return null
  let text = ''
  const m = props.monitor?.(word)
  if (m) text = `${word} = ${m.value} : ${m.type}`
  else if (props.complete) {
    let end = pos
    while (/[A-Za-z0-9_]/.test(src[end] ?? '')) end++
    const last = word.split('.').pop()!.toUpperCase()
    const item = props.complete(src, end)?.items.find((it) => it.label.toUpperCase() === last)
    if (item && item.kind !== 'keyword') text = [item.detail ? `${item.label} : ${item.detail}` : item.label, item.doc].filter(Boolean).join('\n')
  }
  if (!text) return null
  let from = pos
  while (from > 0 && /[A-Za-z0-9_.]/.test(src[from - 1])) from--
  return {
    pos: from,
    above: true,
    create: () => {
      const dom = document.createElement('div')
      dom.className = 'cm-hover'
      dom.textContent = text
      return { dom }
    },
  }
})

// --- Mistakes, underlined, with a quick fix to declare a name.

// declare adds a variable to the file's first VAR section, or a new one after the POU's first
// line, as CODESYS's auto declare does.
function declare(v: EditorView, name: string, type: string) {
  const doc = v.state.doc
  let at = -1
  let indent = '\t'
  for (let n = 1; n <= doc.lines; n++) {
    if (!/^\s*VAR\s*(\(\*.*\*\)|\/\/.*)?$/i.test(doc.line(n).text)) continue
    for (let k = n + 1; k <= doc.lines; k++) {
      const l = doc.line(k)
      if (/^\s*END_VAR\b/i.test(l.text)) {
        at = l.from
        indent = /^\s*/.exec(doc.line(n).text)![0] + '\t'
        break
      }
    }
    break
  }
  let insert = `${indent}${name} : ${type};\n`
  if (at < 0) {
    let header = 1
    for (let n = 1; n <= doc.lines; n++)
      if (/^\s*(PROGRAM|FUNCTION_BLOCK|FUNCTION)\b/i.test(doc.line(n).text)) {
        header = n
        break
      }
    at = doc.line(header).to + 1 > doc.length ? doc.length : doc.line(header).to + 1
    insert = `VAR\n\t${name} : ${type};\nEND_VAR\n`
  }
  v.dispatch({ changes: { from: at, insert }, userEvent: 'input' })
}

function diagnostics(): Diagnostic[] {
  const v = view.value
  if (!v) return []
  const doc = v.state.doc
  return (props.marks ?? []).flatMap((m) => {
    if (m.line < 1 || m.line > doc.lines) return []
    const line = doc.line(m.line)
    const from = Math.min(line.to, line.from + Math.max(0, (m.col ?? 1) - 1))
    let to = from
    while (to < line.to && /[A-Za-z0-9_.%#]/.test(doc.sliceString(to, to + 1))) to++
    if (to === from) to = from === line.to ? from : from + 1
    const undeclared = /^(.+) is not declared$/.exec(m.msg)
    const actions: Action[] =
      undeclared && !props.readOnly ? ['BOOL', 'INT', 'TIME'].map((type) => ({ name: `Declare as ${type}`, apply: (view: EditorView) => declare(view, undeclared[1], type) })) : []
    return [{ from, to, severity: m.severity === 'error' ? 'error' : 'warning', message: m.msg, actions }]
  })
}

watch(
  () => props.marks,
  () => view.value?.dispatch(setDiagnostics(view.value.state, diagnostics())),
)

// --- The editor.

const theme = EditorView.theme({
  '&': { height: '100%', backgroundColor: 'var(--t-s0)', color: 'var(--t-fg)', fontSize: '13px' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': { fontFamily: 'var(--font-mono)', lineHeight: '19px' },
  '.cm-content': { caretColor: 'var(--t-accent)', padding: '8px 0' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--t-accent)', borderLeftWidth: '2px' },
  '.cm-gutters': { backgroundColor: 'var(--t-s0)', color: 'var(--t-faint)', borderRight: '1px solid var(--t-line)' },
  '.cm-activeLine': { backgroundColor: 'var(--t-hover)' },
  '.cm-activeLineGutter': { backgroundColor: 'var(--t-hover)', color: 'var(--t-fg)' },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'var(--t-select)',
  },
  '.cm-selectionMatch': { backgroundColor: 'var(--t-accent-soft)' },
  '.cm-searchMatch': { backgroundColor: 'var(--t-accent-soft)', outline: '1px solid var(--t-accent)' },
  '.cm-searchMatch.cm-searchMatch-selected': { backgroundColor: 'var(--t-select)' },
  '&.cm-focused .cm-matchingBracket': { backgroundColor: 'var(--t-select)', outline: '1px solid var(--t-focus)' },
  '&.cm-focused .cm-nonmatchingBracket': { outline: '1px solid var(--t-danger)' },
  '.cm-foldGutter .cm-gutterElement': { color: 'var(--t-faint)', cursor: 'default' },
  '.cm-foldPlaceholder': { backgroundColor: 'var(--t-s3)', border: 'none', color: 'var(--t-muted)', padding: '0 4px' },
  '.cm-tooltip': { backgroundColor: 'var(--t-s2)', border: '1px solid var(--t-line)', borderRadius: '6px', boxShadow: 'var(--t-shadow)', color: 'var(--t-fg)' },
  '.cm-tooltip-autocomplete > ul': { fontFamily: 'var(--font-mono)', maxHeight: '18em' },
  '.cm-tooltip-autocomplete > ul > li': { padding: '2px 8px 2px 4px', lineHeight: '19px' },
  '.cm-tooltip-autocomplete > ul > li[aria-selected]': { backgroundColor: 'var(--t-highlight)', color: 'var(--t-fg)' },
  '.cm-completionDetail': { color: 'var(--t-muted)', fontStyle: 'normal', marginLeft: '12px' },
  '.cm-completionMatchedText': { textDecoration: 'none', color: 'var(--t-accent-ink)' },
  '.cm-completionInfo': { padding: '6px 8px', maxWidth: '360px', fontFamily: 'var(--font-sans)', fontSize: '12px' },
  '.cm-completionIcon': { width: '3.2em', opacity: '1', paddingRight: '6px', fontSize: '10px', fontFamily: 'var(--font-sans)', color: 'var(--t-muted)', textAlign: 'right' },
  '.cm-completionIcon-local:after, .cm-completionIcon-member:after': { content: '"VAR"' },
  '.cm-completionIcon-io:after': { content: '"I/O"' },
  '.cm-completionIcon-global:after': { content: '"GVL"' },
  '.cm-completionIcon-program:after': { content: '"PRG"' },
  '.cm-completionIcon-fb:after': { content: '"FB"' },
  '.cm-completionIcon-function:after': { content: '"FUN"' },
  '.cm-completionIcon-type:after': { content: '"TYPE"' },
  '.cm-completionIcon-value:after': { content: '"ENUM"' },
  '.cm-completionIcon-keyword:after': { content: '"KW"' },
  '.cm-hover': { padding: '4px 8px', fontFamily: 'var(--font-mono)', fontSize: '12px', whiteSpace: 'pre-wrap', maxWidth: '480px' },
  '.cm-diagnostic': { padding: '4px 8px', fontFamily: 'var(--font-sans)', fontSize: '12px' },
  '.cm-diagnostic-error': { borderLeft: '3px solid var(--t-danger)' },
  '.cm-diagnostic-warning': { borderLeft: '3px solid var(--t-accent)' },
  '.cm-diagnosticAction': { backgroundColor: 'var(--t-s3)', color: 'var(--t-fg)', border: '1px solid var(--t-control)', borderRadius: '4px', padding: '1px 6px' },
  '.cm-lintRange-error': { backgroundImage: 'none', textDecoration: 'underline wavy var(--t-danger)', textUnderlineOffset: '3px' },
  '.cm-lintRange-warning': { backgroundImage: 'none', textDecoration: 'underline wavy var(--t-accent)', textUnderlineOffset: '3px' },
  '.cm-lint-marker': { width: '0.8em', height: '0.8em' },
  '.cm-panels': { backgroundColor: 'var(--t-s1)', color: 'var(--t-fg)' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid var(--t-line)' },
  '.cm-panels.cm-panels-bottom': { borderTop: '1px solid var(--t-line)' },
  '.cm-search': { fontFamily: 'var(--font-sans)', fontSize: '12px', padding: '6px 8px' },
  '.cm-textfield': { backgroundColor: 'var(--t-s0)', color: 'var(--t-fg)', border: '1px solid var(--t-control)', borderRadius: '4px', padding: '2px 6px' },
  '.cm-button': { backgroundImage: 'none', backgroundColor: 'var(--t-s3)', color: 'var(--t-fg)', border: '1px solid var(--t-control)', borderRadius: '4px', padding: '2px 8px' },
  '.cm-panel button[name=close]': { color: 'var(--t-muted)' },
  '.cm-value': {
    display: 'inline-block',
    margin: '0 2px 0 3px',
    padding: '0 4px',
    border: '1px solid var(--t-value-edge)',
    borderRadius: '2px',
    backgroundColor: 'var(--t-value-bg)',
    color: 'var(--t-value-fg)',
    fontSize: '11px',
    lineHeight: '14px',
    verticalAlign: '1px',
    userSelect: 'none',
  },
  '.cm-value.writable': { cursor: 'pointer' },
  '.cm-value.on': { backgroundColor: 'var(--t-value-on)', borderColor: 'var(--t-value-on)', color: 'var(--t-value-on-fg)' },
  '.cm-value.forced:before': {
    content: '"F"',
    backgroundColor: 'var(--t-value-forced)',
    color: 'var(--t-value-on-fg)',
    borderRadius: '2px',
    fontWeight: '700',
    marginRight: '3px',
    padding: '0 2px',
  },
})

let placeTimer = 0
function reportPlace(v: EditorView) {
  window.clearTimeout(placeTimer)
  placeTimer = window.setTimeout(() => emit('place', { cursor: v.state.selection.main.head, top: v.scrollDOM.scrollTop }), 300)
}

onMounted(() => {
  const keys = [
    { key: 'Mod-s', run: () => (emit('save'), true), preventDefault: true },
    { key: 'Shift-Alt-f', run: () => (emit('format'), true), preventDefault: true },
    // Ctrl+Space as in CODESYS; Alt+Space as well, as macOS may keep Ctrl+Space for itself.
    { key: 'Ctrl-Space', run: startCompletion },
    { key: 'Alt-Space', run: startCompletion },
  ]
  const state = EditorState.create({
    doc: props.modelValue,
    selection: props.place ? EditorSelection.cursor(Math.min(props.place.cursor, props.modelValue.length)) : undefined,
    extensions: [
      lineNumbers(),
      foldGutter({ markerDOM: (open) => Object.assign(document.createElement('span'), { textContent: open ? '⌄' : '›' }) }),
      lintGutter(),
      highlightActiveLineGutter(),
      highlightSpecialChars(),
      history(),
      drawSelection(),
      dropCursor(),
      EditorState.allowMultipleSelections.of(true),
      indentOnInput(),
      bracketMatching(),
      closeBrackets(),
      autocompletion({ override: [completions], icons: true }),
      rectangularSelection(),
      crosshairCursor(),
      highlightActiveLine(),
      highlightSelectionMatches(),
      hover,
      EditorState.tabSize.of(4),
      indentUnit.of('\t'),
      structuredText(),
      valuesField,
      readOnlyConf.of(EditorState.readOnly.of(!!props.readOnly)),
      Prec.highest(keymap.of(keys)),
      Prec.high(keymap.of([{ key: 'Enter', run: closeBlock }])),
      keymap.of([...closeBracketsKeymap, ...defaultKeymap, ...searchKeymap, ...historyKeymap, ...foldKeymap, ...completionKeymap, ...lintKeymap, indentWithTab]),
      theme,
      EditorView.contentAttributes.of({ 'aria-label': 'Structured Text', spellcheck: 'false', autocapitalize: 'off', autocorrect: 'off', tabindex: '0' }),
      EditorView.updateListener.of((u) => {
        if (u.docChanged) emit('update:modelValue', u.state.doc.toString())
        if (u.docChanged || u.selectionSet) reportPlace(u.view)
      }),
      EditorView.domEventHandlers({
        mousedown: (e) => {
          const el = (e.target as HTMLElement).closest<HTMLElement>('.cm-value')
          if (!el) return false
          e.preventDefault()
          if (e.detail === 1) clickValue(el)
          return true
        },
        scroll: (_e, v) => reportPlace(v),
      }),
    ],
  })
  const v = new EditorView({ state, parent: host.value! })
  view.value = v
  v.dispatch(setDiagnostics(v.state, diagnostics()))
  if (values.value.length) v.dispatch({ effects: setValues.of(values.value.map((x) => Decoration.widget({ widget: new ValueWidget(x.m, x.prepared), side: 1 }).range(x.at))) })
  if (props.place) requestAnimationFrame(() => (v.scrollDOM.scrollTop = props.place?.top ?? 0))
})

onBeforeUnmount(() => {
  window.clearTimeout(placeTimer)
  view.value?.destroy()
})

// A text changed from outside, such as by Format Document or loading the example, replaces
// what is there; the cursor stays where it can.
watch(
  () => props.modelValue,
  (text) => {
    const v = view.value
    if (!v || text === v.state.doc.toString()) return
    const head = Math.min(v.state.selection.main.head, text.length)
    v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: text }, selection: EditorSelection.cursor(head) })
  },
)

watch(
  () => props.readOnly,
  (ro) => view.value?.dispatch({ effects: readOnlyConf.reconfigure(EditorState.readOnly.of(!!ro)) }),
)

watch(
  () => props.goto?.n,
  () => {
    const v = view.value
    const g = props.goto
    if (!v || !g) return
    const line = v.state.doc.line(Math.min(Math.max(1, g.line), v.state.doc.lines))
    const at = Math.min(line.to, line.from + Math.max(0, g.col - 1))
    v.dispatch({ selection: EditorSelection.cursor(at), effects: EditorView.scrollIntoView(at, { y: 'center' }) })
    v.focus()
  },
)

defineExpose({ focus: () => view.value?.focus() })
</script>

<template>
  <div class="relative flex h-full min-h-0 flex-col bg-s0">
    <div v-if="monitor" class="flex h-7 shrink-0 items-center gap-2 border-0 border-b border-solid border-line bg-s1 px-3 text-xs text-muted">
      <span class="size-1.5 rounded-full bg-ok" />
      <span v-if="showValues">Values of <span class="font-mono text-fg">{{ scope || 'the global variables' }}</span></span>
      <span v-else>Online, values hidden</span>
      <span class="flex-1" />
      <button class="t-btn h-5 px-2 text-[11px]" :aria-pressed="showValues" @click="showValues = !showValues">{{ showValues ? 'Hide values' : 'Show values' }}</button>
    </div>
    <div ref="host" class="relative min-h-0 flex-1 overflow-hidden focus-within:outline-2 focus-within:-outline-offset-2 focus-within:outline-focus" />
    <div v-if="prep" class="absolute z-10" :style="{ left: `${prep.x}px`, top: `${prep.y}px` }">
      <input
        ref="prepInput"
        v-model="prep.text"
        class="t-input h-6 w-36 px-1.5 font-mono text-xs"
        aria-label="Prepared value"
        @keydown.enter.prevent="prepareDone(true)"
        @keydown.esc.prevent="prepareDone(false)"
        @blur="prepareDone(false)"
      />
    </div>
  </div>
</template>
