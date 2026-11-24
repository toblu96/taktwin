import { foldService, HighlightStyle, indentService, StreamLanguage, syntaxHighlighting, type StringStream } from '@codemirror/language'
import { EditorSelection, type Extension } from '@codemirror/state'
import type { Command } from '@codemirror/view'
import { tags } from '@lezer/highlight'

// Structured Text for CodeMirror: the colours of its words, where a new line starts, which
// blocks fold, its comments, and a block closed as it is opened, as CODESYS's smart coding
// does: Enter after IF … THEN puts END_IF below.

const words = (s: string) => new Set(s.split(/\s+/).filter(Boolean))

const KEYWORDS = words(`PROGRAM END_PROGRAM FUNCTION END_FUNCTION FUNCTION_BLOCK END_FUNCTION_BLOCK VAR VAR_INPUT VAR_OUTPUT VAR_IN_OUT
  VAR_TEMP VAR_GLOBAL VAR_EXTERNAL VAR_STAT END_VAR CONSTANT RETAIN NON_RETAIN PERSISTENT AT TYPE END_TYPE STRUCT END_STRUCT ARRAY OF
  IF THEN ELSIF ELSE END_IF CASE END_CASE FOR TO BY DO END_FOR WHILE END_WHILE REPEAT UNTIL END_REPEAT EXIT CONTINUE RETURN AND OR XOR
  NOT MOD`)
const TYPES = words(`BOOL SINT INT DINT LINT USINT UINT UDINT ULINT BYTE WORD DWORD LWORD REAL LREAL TIME STRING WSTRING TON TOF TP
  R_TRIG F_TRIG CTU CTD CTUD SR RS`)
const FUNCTIONS = words(`ABS SQRT LN LOG EXP SIN COS TAN ASIN ACOS ATAN EXPT TRUNC MOVE SEL MAX MIN LIMIT MUX SHL SHR ROL ROR LEN
  LEFT RIGHT MID CONCAT INSERT DELETE REPLACE FIND`)
const conversion = /^([A-Z]+_TO_[A-Z]+|TO_[A-Z]+|TRUNC_[A-Z]+)$/

interface State {
  // depth is how many (* *) comments are open, which nest.
  depth: number
  // brace is set inside a { } pragma.
  brace: boolean
}

function comment(stream: StringStream, state: State): string {
  while (!stream.eol()) {
    if (stream.match('(*')) state.depth++
    else if (stream.match('*)')) {
      if (--state.depth === 0) break
    } else stream.next()
  }
  return 'comment'
}

const st = StreamLanguage.define<State>({
  name: 'st',
  startState: () => ({ depth: 0, brace: false }),
  copyState: (s) => ({ ...s }),
  token(stream, state) {
    if (state.depth > 0) return comment(stream, state)
    if (state.brace) {
      if (stream.skipTo('}')) {
        stream.next()
        state.brace = false
      } else stream.skipToEnd()
      return 'comment'
    }
    if (stream.eatSpace()) return null
    if (stream.match('(*')) {
      state.depth = 1
      return comment(stream, state)
    }
    if (stream.match('//')) {
      stream.skipToEnd()
      return 'comment'
    }
    if (stream.eat('{')) {
      state.brace = true
      if (stream.skipTo('}')) {
        stream.next()
        state.brace = false
      } else stream.skipToEnd()
      return 'comment'
    }
    const quote = stream.peek()
    if (quote === "'" || quote === '"') {
      stream.next()
      while (!stream.eol()) {
        const c = stream.next()
        if (c === '$') stream.next()
        else if (c === quote) break
      }
      return 'string'
    }
    if (stream.match(/^%[A-Za-z]+[0-9.*]*/)) return 'variableName.special'
    if (stream.match(/^[0-9][0-9_]*(#[0-9A-Fa-f_]+)?(\.[0-9_]+)?([eE][+-]?[0-9]+)?/)) return 'number'
    const word = stream.match(/^[A-Za-z_][A-Za-z0-9_]*/) as RegExpMatchArray | null
    if (word) {
      const up = word[0].toUpperCase()
      if (stream.peek() === '#') {
        // A typed literal: T#1s, INT#5, Colour#Red.
        stream.match(/^#[-+]?[A-Za-z0-9_.:#]*/)
        return 'number'
      }
      if (up === 'TRUE' || up === 'FALSE') return 'bool'
      if (KEYWORDS.has(up)) return 'keyword'
      if (TYPES.has(up)) return 'typeName'
      if (FUNCTIONS.has(up) || conversion.test(up)) return 'variableName.function'
      return null
    }
    stream.next()
    return null
  },
  languageData: {
    commentTokens: { line: '//', block: { open: '(*', close: '*)' } },
    // Typing the end of a block puts it where its start is.
    indentOnInput: /^\s*(END_[A-Za-z_]+|ELSE|ELSIF|UNTIL)$/i,
    closeBrackets: { brackets: ['(', '[', "'", '"'] },
  },
})

const highlighting = HighlightStyle.define([
  { tag: tags.keyword, color: 'var(--t-code-keyword)', fontWeight: '600' },
  { tag: tags.typeName, color: 'var(--t-code-type)' },
  { tag: tags.function(tags.variableName), color: 'var(--t-code-function)' },
  { tag: tags.comment, color: 'var(--t-code-comment)', fontStyle: 'italic' },
  { tag: tags.string, color: 'var(--t-code-string)' },
  { tag: [tags.number, tags.bool], color: 'var(--t-code-number)' },
  { tag: tags.special(tags.variableName), color: 'var(--t-code-address)' },
])

// What a line opens, and the word that ends it.
const BLOCKS: [RegExp, string][] = [
  [/^\s*IF\b.*\bTHEN\s*(\(\*.*\*\)|\/\/.*)?$/i, 'END_IF'],
  [/^\s*CASE\b.*\bOF\s*(\(\*.*\*\)|\/\/.*)?$/i, 'END_CASE'],
  [/^\s*FOR\b.*\bDO\s*(\(\*.*\*\)|\/\/.*)?$/i, 'END_FOR'],
  [/^\s*WHILE\b.*\bDO\s*(\(\*.*\*\)|\/\/.*)?$/i, 'END_WHILE'],
  [/^\s*REPEAT\s*(\(\*.*\*\)|\/\/.*)?$/i, 'END_REPEAT'],
  [/^\s*VAR(_INPUT|_OUTPUT|_IN_OUT|_TEMP|_GLOBAL|_EXTERNAL|_STAT)?(\s+(CONSTANT|RETAIN|NON_RETAIN|PERSISTENT))*\s*(\(\*.*\*\)|\/\/.*)?$/i, 'END_VAR'],
  [/^.*\bSTRUCT\s*(\(\*.*\*\)|\/\/.*)?$/i, 'END_STRUCT'],
  [/^\s*TYPE\s*(\(\*.*\*\)|\/\/.*)?$/i, 'END_TYPE'],
]

// What lines in a block start with besides its end: an IF's ELSIF and ELSE, a REPEAT's UNTIL.
const SIBLINGS: Record<string, RegExp> = { END_IF: /^(ELSIF|ELSE)\b/i, END_REPEAT: /^UNTIL\b/i, END_CASE: /^ELSE\b/i }

const indentOf = (text: string) => /^\s*/.exec(text)![0]
const width = (indent: string, tab: number) => [...indent].reduce((n, c) => (c === '\t' ? n + tab - (n % tab) : n + 1), 0)

// OPENERS are what a following line indents after: a block's start, or a CASE label.
const OPENERS =
  /^\s*(.*\b(THEN|ELSE|DO|OF|REPEAT|VAR|VAR_INPUT|VAR_OUTPUT|VAR_IN_OUT|VAR_TEMP|VAR_GLOBAL|VAR_EXTERNAL|VAR_STAT|STRUCT|TYPE)\b\s*(\(\*.*\*\))?\s*$|[^:=]*[^:=\s]\s*:\s*(\(\*.*\*\))?\s*$)/i
const CLOSERS = /^\s*(END_[A-Za-z_]+|ELSE|ELSIF|UNTIL)\b/i

// closeBlock is Enter at the end of a line that opens a block not closed yet: an empty line to
// write on, and the block's end below, as CODESYS's Smart AutoIndent with code completion does.
const closeBlock: Command = (view) => {
  const { state } = view
  const sel = state.selection.main
  if (!sel.empty || state.readOnly) return false
  const line = state.doc.lineAt(sel.head)
  if (sel.head !== line.to) return false
  const end = BLOCKS.find(([re]) => re.test(line.text))?.[1]
  if (!end) return false
  const tab = state.tabSize
  const indent = indentOf(line.text)
  // The block is closed already when the next line no deeper starts with its end, or with an
  // ELSE or ELSIF of it.
  for (let n = line.number + 1; n <= state.doc.lines; n++) {
    const next = state.doc.line(n).text
    if (!next.trim()) continue
    if (width(indentOf(next), tab) > width(indent, tab)) continue
    const rest = next.trim()
    if (rest.toUpperCase().startsWith(end) || SIBLINGS[end]?.test(rest)) return false
    break
  }
  const insert = `\n${indent}\t\n${indent}${end}`
  view.dispatch({
    changes: { from: sel.head, insert },
    selection: EditorSelection.cursor(sel.head + indent.length + 2),
    scrollIntoView: true,
    userEvent: 'input',
  })
  return true
}

// Where a new line starts: a level deeper after a block's start or a CASE label, a level less
// for the end of a block, and else as the line before.
const indent = indentService.of((cx, pos) => {
  const line = cx.state.doc.lineAt(pos)
  let n = line.number - 1
  while (n > 0 && !cx.state.doc.line(n).text.trim()) n--
  if (n < 1) return 0
  const prev = cx.state.doc.line(n)
  let level = cx.lineIndent(prev.from)
  if (OPENERS.test(prev.text) && !/^\s*END_/i.test(prev.text)) level += cx.unit
  if (CLOSERS.test(cx.textAfterPos(pos))) level -= cx.unit
  return Math.max(0, level)
})

const START = /^\s*(IF|CASE|FOR|WHILE|REPEAT|VAR(_[A-Z_]+)?|TYPE|PROGRAM|FUNCTION_BLOCK|FUNCTION)\b|\bSTRUCT\s*$/i

// A block folds from its first line to the line before its end; a comment over several
// lines folds too.
const folding = foldService.of((state, from, to) => {
  const line = state.doc.lineAt(from)
  const text = line.text
  if (/^\s*\(\*/.test(text) && !text.includes('*)')) {
    for (let n = line.number + 1; n <= state.doc.lines; n++) {
      const l = state.doc.line(n)
      if (l.text.includes('*)')) return n > line.number ? { from: to, to: l.to } : null
    }
    return null
  }
  if (!START.test(text) || /\bEND_[A-Z_]+\s*;?\s*(\(\*.*\*\))?\s*$/i.test(text)) return null
  const tab = state.tabSize
  const indent = width(indentOf(text), tab)
  for (let n = line.number + 1; n <= state.doc.lines; n++) {
    const l = state.doc.line(n)
    if (!l.text.trim()) continue
    if (width(indentOf(l.text), tab) <= indent) {
      if (!/^\s*(END_|UNTIL\b)/i.test(l.text)) return null
      const last = state.doc.line(n - 1)
      return last.number > line.number ? { from: to, to: last.to } : null
    }
  }
  return null
})

// structuredText is the language with its colours, indenting and folding.
export function structuredText(): Extension {
  return [st, syntaxHighlighting(highlighting), indent, folding]
}

export { closeBlock }
