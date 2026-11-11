// Completion for the ST editor, as CODESYS lists components on Ctrl+Space: the variables of
// the POU at the cursor, the global variables and the I/O list, the POUs, types, standard
// functions and keywords, and after a dot the members of what is before it.
import { tokenize, type Token } from './st'

export interface Decl {
  name: string
  type: string
  section: string
  comment: string
  at: string
}

export interface Pou {
  kind: 'PROGRAM' | 'FUNCTION_BLOCK' | 'FUNCTION'
  name: string
  result: string
  start: number
  vars: Decl[]
}

export interface TypeDef {
  name: string
  kind: 'STRUCT' | 'ENUM' | 'ALIAS'
  fields: Decl[]
  values: string[]
  alias: string
}

// Unit is what a file declares.
export interface Unit {
  file: string
  pous: Pou[]
  globals: Decl[]
  types: TypeDef[]
}

export type ItemKind = 'local' | 'member' | 'io' | 'global' | 'program' | 'fb' | 'function' | 'type' | 'value' | 'keyword'

export interface Item {
  label: string
  kind: ItemKind
  detail: string
  doc: string
}

export interface Completion {
  from: number
  items: Item[]
}

const up = (t: Token | undefined) => t?.text.toUpperCase() ?? ''
const VAR_SECTIONS = /^VAR(_INPUT|_OUTPUT|_IN_OUT|_TEMP|_GLOBAL|_EXTERNAL|_STAT)?$/
const END = /^END_(VAR|STRUCT|TYPE|PROGRAM|FUNCTION|FUNCTION_BLOCK)$/

function commentText(c: string): string {
  return c
    .replace(/^\(\*|\*\)$/g, '')
    .replace(/^\/\//, '')
    .replace(/^\{|\}$/g, '')
    .trim()
}

// parseUnit reads the declarations of a file: its POUs with their variables, the global
// variables and the types. The comment after a declaration on its line is its description.
export function parseUnit(file: string, src: string): Unit {
  const all = tokenize(src)
  const sig: { t: Token; i: number }[] = []
  all.forEach((t, i) => {
    if (t.kind !== 'space' && t.kind !== 'comment') sig.push({ t, i })
  })
  const unit: Unit = { file, pous: [], globals: [], types: [] }
  const n = sig.length
  const at = (j: number) => sig[j]?.t
  let pou: Pou | null = null
  let section = ''
  let inType = false
  let struct: TypeDef | null = null

  // The comment after a ; on the same line.
  const trailing = (j: number) => {
    for (let k = sig[j].i + 1; k < all.length; k++) {
      const t = all[k]
      if (t.kind === 'space') {
        if (t.text.includes('\n')) return ''
        continue
      }
      return t.kind === 'comment' ? commentText(t.text) : ''
    }
    return ''
  }

  // decl reads "a, b AT %IX0.0 : TYPE := init;" from j, into list; it gives where it ends.
  const decl = (j: number, list: Decl[], sect: string): number => {
    const names: string[] = []
    while (at(j)?.kind === 'ident') {
      names.push(at(j)!.text)
      j++
      if (at(j)?.text !== ',') break
      j++
    }
    let address = ''
    if (up(at(j)) === 'AT') {
      address = at(j + 1)?.text ?? ''
      j += 2
    }
    if (at(j)?.text !== ':' || !names.length) return j
    j++
    const from = j
    let depth = 0
    while (j < n) {
      const t = at(j)!
      if (depth === 0 && (t.text === ';' || t.text === ':=' || END.test(up(t)))) break
      if (t.text === '(' || t.text === '[') depth++
      if (t.text === ')' || t.text === ']') depth--
      j++
    }
    const type = j > from ? src.slice(at(from)!.start, at(j - 1)!.start + at(j - 1)!.text.length).replace(/\s+/g, ' ') : ''
    if (at(j)?.text === ':=') {
      depth = 0
      while (j < n && !(depth === 0 && at(j)!.text === ';') && !END.test(up(at(j)))) {
        if (at(j)!.text === '(' || at(j)!.text === '[') depth++
        if (at(j)!.text === ')' || at(j)!.text === ']') depth--
        j++
      }
    }
    const comment = at(j)?.text === ';' ? trailing(j) : ''
    for (const name of names) list.push({ name, type, section: sect, comment, at: address })
    return at(j)?.text === ';' ? j + 1 : j
  }

  let j = 0
  while (j < n) {
    const t = at(j)!
    const word = t.kind === 'keyword' ? up(t) : ''
    if (struct) {
      if (word === 'END_STRUCT') {
        struct = null
        j++
      } else if (t.kind === 'ident') j = decl(j, struct.fields, 'STRUCT')
      else j++
      continue
    }
    if (section) {
      if (word === 'END_VAR') section = ''
      if (t.kind === 'ident') {
        j = decl(j, section === 'VAR_GLOBAL' ? unit.globals : (pou?.vars ?? unit.globals), section)
        continue
      }
      j++
      continue
    }
    if (word === 'PROGRAM' || word === 'FUNCTION_BLOCK' || word === 'FUNCTION') {
      const name = at(j + 1)?.kind === 'ident' ? at(j + 1)!.text : ''
      pou = { kind: word, name, result: '', start: t.start, vars: [] }
      if (word === 'FUNCTION' && at(j + 2)?.text === ':') pou.result = at(j + 3)?.text ?? ''
      unit.pous.push(pou)
      j += 2
    } else if (VAR_SECTIONS.test(word)) {
      section = word
      j++
    } else if (word === 'TYPE') {
      inType = true
      j++
    } else if (word === 'END_TYPE') {
      inType = false
      j++
    } else if (inType && t.kind === 'ident' && at(j + 1)?.text === ':') {
      const name = t.text
      if (up(at(j + 2)) === 'STRUCT') {
        struct = { name, kind: 'STRUCT', fields: [], values: [], alias: '' }
        unit.types.push(struct)
        j += 3
      } else if (at(j + 2)?.text === '(') {
        const e: TypeDef = { name, kind: 'ENUM', fields: [], values: [], alias: '' }
        j += 3
        while (j < n && at(j)!.text !== ')') {
          if (at(j)!.kind === 'ident' && at(j - 1)?.text !== ':=') e.values.push(at(j)!.text)
          j++
        }
        unit.types.push(e)
      } else {
        const list: Decl[] = []
        j = decl(j, list, 'TYPE')
        unit.types.push({ name, kind: 'ALIAS', fields: [], values: [], alias: list[0]?.type ?? '' })
      }
    } else j++
  }
  return unit
}

// The standard function blocks with their inputs and outputs.
const STD_FB: Record<string, [string, string, string][]> = {}
const timer: [string, string, string][] = [
  ['IN', 'BOOL', 'VAR_INPUT'],
  ['PT', 'TIME', 'VAR_INPUT'],
  ['Q', 'BOOL', 'VAR_OUTPUT'],
  ['ET', 'TIME', 'VAR_OUTPUT'],
]
STD_FB.TON = STD_FB.TOF = STD_FB.TP = timer
STD_FB.R_TRIG = STD_FB.F_TRIG = [
  ['CLK', 'BOOL', 'VAR_INPUT'],
  ['Q', 'BOOL', 'VAR_OUTPUT'],
]
STD_FB.CTU = [
  ['CU', 'BOOL', 'VAR_INPUT'],
  ['RESET', 'BOOL', 'VAR_INPUT'],
  ['PV', 'INT', 'VAR_INPUT'],
  ['Q', 'BOOL', 'VAR_OUTPUT'],
  ['CV', 'INT', 'VAR_OUTPUT'],
]
STD_FB.CTD = [
  ['CD', 'BOOL', 'VAR_INPUT'],
  ['LOAD', 'BOOL', 'VAR_INPUT'],
  ['PV', 'INT', 'VAR_INPUT'],
  ['Q', 'BOOL', 'VAR_OUTPUT'],
  ['CV', 'INT', 'VAR_OUTPUT'],
]
STD_FB.CTUD = [
  ['CU', 'BOOL', 'VAR_INPUT'],
  ['CD', 'BOOL', 'VAR_INPUT'],
  ['RESET', 'BOOL', 'VAR_INPUT'],
  ['LOAD', 'BOOL', 'VAR_INPUT'],
  ['PV', 'INT', 'VAR_INPUT'],
  ['QU', 'BOOL', 'VAR_OUTPUT'],
  ['QD', 'BOOL', 'VAR_OUTPUT'],
  ['CV', 'INT', 'VAR_OUTPUT'],
]
STD_FB.SR = [
  ['SET1', 'BOOL', 'VAR_INPUT'],
  ['RESET', 'BOOL', 'VAR_INPUT'],
  ['Q1', 'BOOL', 'VAR_OUTPUT'],
]
STD_FB.RS = [
  ['SET', 'BOOL', 'VAR_INPUT'],
  ['RESET1', 'BOOL', 'VAR_INPUT'],
  ['Q1', 'BOOL', 'VAR_OUTPUT'],
]

const STD_FB_DOC: Record<string, string> = {
  TON: 'On delay: Q follows IN after PT',
  TOF: 'Off delay: Q stays on for PT after IN goes off',
  TP: 'Pulse: Q is on for PT from a rising edge of IN',
  R_TRIG: 'Q is on for one cycle on a rising edge of CLK',
  F_TRIG: 'Q is on for one cycle on a falling edge of CLK',
  CTU: 'Counts up on rising edges of CU; Q when CV >= PV',
  CTD: 'Counts down on rising edges of CD; Q when CV <= 0',
  CTUD: 'Counts up and down',
  SR: 'Flip-flop, set dominant',
  RS: 'Flip-flop, reset dominant',
}

const ELEMENTARY = 'BOOL SINT INT DINT LINT USINT UINT UDINT ULINT BYTE WORD DWORD LWORD REAL LREAL TIME STRING WSTRING ARRAY'.split(' ')

const STD_FUNCTIONS: Record<string, string> = {
  ABS: 'ABS(IN)',
  SQRT: 'SQRT(IN)',
  LN: 'LN(IN)',
  LOG: 'LOG(IN)',
  EXP: 'EXP(IN)',
  SIN: 'SIN(IN)',
  COS: 'COS(IN)',
  TAN: 'TAN(IN)',
  ASIN: 'ASIN(IN)',
  ACOS: 'ACOS(IN)',
  ATAN: 'ATAN(IN)',
  EXPT: 'EXPT(IN1, IN2): IN1 to the power of IN2',
  TRUNC: 'TRUNC(IN)',
  MOVE: 'MOVE(IN)',
  SEL: 'SEL(G, IN0, IN1): IN1 if G, else IN0',
  MAX: 'MAX(IN1, IN2, …)',
  MIN: 'MIN(IN1, IN2, …)',
  LIMIT: 'LIMIT(MN, IN, MX)',
  MUX: 'MUX(K, IN0, IN1, …)',
  SHL: 'SHL(IN, N)',
  SHR: 'SHR(IN, N)',
  ROL: 'ROL(IN, N)',
  ROR: 'ROR(IN, N)',
  LEN: 'LEN(STR)',
  LEFT: 'LEFT(STR, SIZE)',
  RIGHT: 'RIGHT(STR, SIZE)',
  MID: 'MID(STR, LEN, POS)',
  CONCAT: 'CONCAT(STR1, STR2, …)',
  INSERT: 'INSERT(STR1, STR2, POS)',
  DELETE: 'DELETE(STR, LEN, POS)',
  REPLACE: 'REPLACE(STR1, STR2, LEN, POS)',
  FIND: 'FIND(STR1, STR2): where STR2 is in STR1, 0 if not',
  INT_TO_REAL: 'INT_TO_REAL(IN); any A_TO_B converts',
  REAL_TO_INT: 'REAL_TO_INT(IN), rounded',
  TIME_TO_DINT: 'TIME_TO_DINT(IN), in ms',
  DINT_TO_TIME: 'DINT_TO_TIME(IN), from ms',
  BOOL_TO_INT: 'BOOL_TO_INT(IN)',
}

const KEYWORDS = `IF THEN ELSIF ELSE END_IF CASE OF END_CASE FOR TO BY DO END_FOR WHILE END_WHILE REPEAT UNTIL END_REPEAT EXIT CONTINUE RETURN
  AND OR XOR NOT MOD TRUE FALSE VAR VAR_INPUT VAR_OUTPUT VAR_IN_OUT VAR_TEMP VAR_GLOBAL END_VAR CONSTANT RETAIN AT`.split(/\s+/)

const listName = (file: string) => file.replace(/\.st$/i, '')
const same = (a: string, b: string) => a.toUpperCase() === b.toUpperCase()

const declItem = (d: Decl, kind: ItemKind): Item => ({
  label: d.name,
  kind,
  detail: d.at ? `${d.type} AT ${d.at}` : d.type,
  doc: d.comment || (d.section && d.section !== 'VAR' && d.section !== 'STRUCT' ? d.section : ''),
})

// inDeclarations tells whether an offset is inside a VAR block or a TYPE, where a name is
// followed by its type.
function inDeclarations(src: string, offset: number): boolean {
  let depth = 0
  for (const t of tokenize(src.slice(0, offset))) {
    if (t.kind !== 'keyword') continue
    const w = t.text.toUpperCase()
    if (VAR_SECTIONS.test(w) || w === 'TYPE') depth++
    else if (w === 'END_VAR' || w === 'END_TYPE') depth = Math.max(0, depth - 1)
  }
  return depth > 0
}

// complete lists what fits at an offset of a file, given what every file of the project
// declares, the file's own as it is now among them.
export function complete(units: Unit[], file: string, src: string, offset: number): Completion | null {
  const before = src.slice(0, offset)
  // Nothing in a comment or a string.
  for (const t of tokenize(before)) if ((t.kind === 'comment' || t.kind === 'string') && t.start + t.text.length >= offset && offset > t.start) return null
  const prefix = /[A-Za-z_][A-Za-z0-9_]*$/.exec(before)?.[0] ?? ''
  const from = offset - prefix.length
  const head = before.slice(0, from)

  const unit = units.find((u) => u.file === file)
  const pou = unit?.pous.filter((p) => p.start <= offset).pop() ?? null
  const pous = units.flatMap((u) => u.pous)
  const types = units.flatMap((u) => u.types)
  const lists = units.filter((u) => u.globals.length)
  const globals = lists.flatMap((u) => u.globals.map((g) => ({ g, list: u.file })))

  const items: Item[] = []

  const chain = /((?:[A-Za-z_][A-Za-z0-9_]*(?:[ \t]*\[[^\]\n]*\])*[ \t]*\.[ \t]*)+)$/.exec(head)
  if (chain) {
    const parts = chain[1]
      .split('.')
      .map((s) => s.trim())
      .filter(Boolean)
    for (const m of members(parts)) items.push(m)
  } else if (/:\s*$|\bOF\s*$/i.test(head) && inDeclarations(src, from)) {
    for (const t of ELEMENTARY) items.push({ label: t, kind: 'type', detail: 'type', doc: '' })
    for (const t of Object.keys(STD_FB)) items.push({ label: t, kind: 'fb', detail: 'FUNCTION_BLOCK', doc: STD_FB_DOC[t] })
    for (const p of pous) if (p.kind === 'FUNCTION_BLOCK') items.push({ label: p.name, kind: 'fb', detail: 'FUNCTION_BLOCK', doc: '' })
    for (const t of types) items.push({ label: t.name, kind: 'type', detail: t.kind, doc: t.alias })
  } else {
    if (pou) {
      for (const v of pou.vars) items.push(declItem(v, 'local'))
      if (pou.kind === 'FUNCTION') items.push({ label: pou.name, kind: 'local', detail: pou.result, doc: 'the result of the function' })
    }
    for (const { g, list } of globals) {
      const it = declItem(g, g.at ? 'io' : 'global')
      it.doc = [g.comment, `in ${listName(list)}`].filter(Boolean).join(' · ')
      items.push(it)
    }
    for (const p of pous) {
      const ins = p.vars.filter((v) => v.section === 'VAR_INPUT' || v.section === 'VAR_IN_OUT').map((v) => `${v.name} : ${v.type}`)
      const kind: ItemKind = p.kind === 'PROGRAM' ? 'program' : p.kind === 'FUNCTION_BLOCK' ? 'fb' : 'function'
      items.push({ label: p.name, kind, detail: p.kind === 'FUNCTION' ? `FUNCTION : ${p.result}` : p.kind, doc: ins.length ? `(${ins.join(', ')})` : '' })
    }
    for (const l of lists) items.push({ label: listName(l.file), kind: 'global', detail: 'global variable list', doc: '' })
    for (const t of types) items.push({ label: t.name, kind: 'type', detail: t.kind, doc: t.kind === 'ENUM' ? t.values.join(', ') : '' })
    for (const [f, sig] of Object.entries(STD_FUNCTIONS)) items.push({ label: f, kind: 'function', detail: 'function', doc: sig })
    for (const k of KEYWORDS) items.push({ label: k, kind: 'keyword', detail: '', doc: '' })
  }

  return { from, items: rank(items, prefix) }

  // members lists what follows a dotted name: the variables of a list or a program, the
  // inputs and outputs of an instance, the fields of a struct, or the values of an enum.
  function members(parts: string[]): Item[] {
    const first = parts[0].replace(/\[.*$/, '').trim()
    let decls: Decl[] | null = null
    let type = ''
    const list = lists.find((l) => same(listName(l.file), first))
    const prog = pous.find((p) => p.kind === 'PROGRAM' && same(p.name, first))
    const enumType = types.find((t) => t.kind === 'ENUM' && same(t.name, first))
    if (parts.length === 1 && enumType) return enumType.values.map((v) => ({ label: v, kind: 'value', detail: enumType.name, doc: '' }))
    if (list) decls = list.globals
    else if (prog) decls = prog.vars
    else {
      const v = pou?.vars.find((d) => same(d.name, first)) ?? globals.find(({ g }) => same(g.name, first))?.g
      if (!v) return []
      type = indexed(v.type, parts[0])
    }
    for (const part of parts.slice(1)) {
      const name = part.replace(/\[.*$/, '').trim()
      const list = decls ?? membersOf(type)
      const d = list.find((x) => same(x.name, name))
      if (!d) return []
      decls = null
      type = indexed(d.type, part)
    }
    const out = decls ?? membersOf(type)
    return out.map((d) => declItem(d, 'member'))
  }

  // indexed is the type of a name, or of an element where it is indexed.
  function indexed(type: string, part: string): string {
    let t = type
    for (let k = 0; k < (part.match(/\[/g)?.length ?? 0); k++) t = /\bOF\s+(.+)$/i.exec(t)?.[1] ?? t
    return t.trim()
  }

  function membersOf(type: string): Decl[] {
    const std = STD_FB[type.toUpperCase()]
    if (std) return std.map(([name, t, section]) => ({ name, type: t, section, comment: '', at: '' }))
    const fb = pous.find((p) => p.kind === 'FUNCTION_BLOCK' && same(p.name, type))
    if (fb) return fb.vars.filter((v) => v.section === 'VAR_INPUT' || v.section === 'VAR_OUTPUT')
    const t = types.find((x) => same(x.name, type))
    if (t?.kind === 'STRUCT') return t.fields
    if (t?.kind === 'ALIAS') return membersOf(t.alias)
    return []
  }
}

// rank keeps the items whose name starts with the prefix, then those that contain it, each
// in the order given, without doubles.
function rank(items: Item[], prefix: string): Item[] {
  const p = prefix.toUpperCase()
  const seen = new Set<string>()
  const starts: Item[] = []
  const contains: Item[] = []
  for (const it of items) {
    const l = it.label.toUpperCase()
    if (seen.has(l)) continue
    if (l.startsWith(p)) starts.push(it)
    else if (p && l.includes(p)) contains.push(it)
    else continue
    seen.add(l)
  }
  return [...starts, ...contains]
}
