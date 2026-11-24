// Structured Text for the editor: its tokens, the name at a place, and what a file declares.

const KEYWORDS = new Set(
  `PROGRAM END_PROGRAM FUNCTION END_FUNCTION FUNCTION_BLOCK END_FUNCTION_BLOCK VAR VAR_INPUT VAR_OUTPUT VAR_IN_OUT VAR_TEMP VAR_GLOBAL
  VAR_EXTERNAL VAR_STAT END_VAR CONSTANT RETAIN NON_RETAIN PERSISTENT AT TYPE END_TYPE STRUCT END_STRUCT ARRAY OF IF THEN ELSIF ELSE
  END_IF CASE END_CASE FOR TO BY DO END_FOR WHILE END_WHILE REPEAT UNTIL END_REPEAT EXIT CONTINUE RETURN AND OR XOR NOT MOD`.split(/\s+/),
)

const TYPES = new Set(
  `BOOL SINT INT DINT LINT USINT UINT UDINT ULINT BYTE WORD DWORD LWORD REAL LREAL TIME STRING WSTRING TON TOF TP R_TRIG F_TRIG
  CTU CTD CTUD SR RS`.split(/\s+/),
)

const FUNCTIONS = new Set(
  `ABS SQRT LN LOG EXP SIN COS TAN ASIN ACOS ATAN EXPT TRUNC MOVE SEL MAX MIN LIMIT MUX SHL SHR ROL ROR LEN LEFT RIGHT MID CONCAT
  INSERT DELETE REPLACE FIND ADD SUB MUL DIV GT GE EQ LE LT NE`.split(/\s+/),
)

export type TokenKind = 'keyword' | 'type' | 'function' | 'comment' | 'string' | 'number' | 'address' | 'bool' | 'ident' | 'op' | 'space'

export interface Token {
  kind: TokenKind
  text: string
  start: number
}

const isConversion = (w: string) => /^([A-Z]+_TO_[A-Z]+|TO_[A-Z]+|TRUNC_[A-Z]+)$/.test(w)

// tokenize splits source into tokens, keeping every character, so that the highlighted text
// lines up with what is typed. An unclosed comment or string runs to the end.
export function tokenize(src: string): Token[] {
  const out: Token[] = []
  let i = 0
  const push = (kind: TokenKind, end: number) => {
    out.push({ kind, text: src.slice(i, end), start: i })
    i = end
  }
  while (i < src.length) {
    const c = src[i]
    if (/\s/.test(c)) {
      let j = i
      while (j < src.length && /\s/.test(src[j])) j++
      push('space', j)
    } else if (c === '(' && src[i + 1] === '*') {
      let depth = 0
      let j = i
      while (j < src.length) {
        if (src[j] === '(' && src[j + 1] === '*') {
          depth++
          j += 2
        } else if (src[j] === '*' && src[j + 1] === ')') {
          j += 2
          if (--depth === 0) break
        } else j++
      }
      push('comment', j)
    } else if (c === '/' && src[i + 1] === '/') {
      const j = src.indexOf('\n', i)
      push('comment', j < 0 ? src.length : j)
    } else if (c === '{') {
      const j = src.indexOf('}', i)
      push('comment', j < 0 ? src.length : j + 1)
    } else if (c === "'" || c === '"') {
      let j = i + 1
      while (j < src.length && src[j] !== c && src[j] !== '\n') j += src[j] === '$' ? 2 : 1
      push('string', Math.min(src.length, j + 1))
    } else if (c === '%') {
      const m = /^%[A-Za-z]+[0-9.*]*/.exec(src.slice(i))
      push('address', i + (m ? m[0].length : 1))
    } else if (/[0-9]/.test(c)) {
      const m = /^[0-9_]+(#[0-9A-Fa-f_]+)?(\.[0-9_]+)?([eE][+-]?[0-9]+)?/.exec(src.slice(i))
      push('number', i + (m ? m[0].length : 1))
    } else if (/[A-Za-z_]/.test(c)) {
      const m = /^[A-Za-z_][A-Za-z0-9_]*/.exec(src.slice(i))!
      let end = i + m[0].length
      const word = m[0].toUpperCase()
      if (src[end] === '#') {
        // A typed literal: T#1s, INT#5, Colour#Red.
        const v = /^#[-+]?[A-Za-z0-9_.:#]*/.exec(src.slice(end))!
        end += v[0].length
        push('number', end)
      } else if (word === 'TRUE' || word === 'FALSE') push('bool', end)
      else if (KEYWORDS.has(word)) push('keyword', end)
      else if (TYPES.has(word)) push('type', end)
      else if (FUNCTIONS.has(word) || isConversion(word)) push('function', end)
      else push('ident', end)
    } else {
      const two = src.slice(i, i + 2)
      push('op', i + ([':=', '=>', '<>', '<=', '>=', '**', '..'].includes(two) ? 2 : 1))
    }
  }
  return out
}

// declared names what a file declares first: a POU and its name, or a global variable list.
export function declared(src: string): { kind: string; name: string } | null {
  for (const t of tokenize(src)) {
    if (t.kind === 'keyword' && /^(PROGRAM|FUNCTION_BLOCK|FUNCTION)$/i.test(t.text)) {
      const rest = src.slice(t.start + t.text.length)
      const m = /^\s+([A-Za-z_][A-Za-z0-9_]*)/.exec(rest)
      return { kind: t.text.toUpperCase(), name: m ? m[1] : '' }
    }
    if (t.kind === 'keyword' && /^VAR_GLOBAL$/i.test(t.text)) return { kind: 'GVL', name: '' }
    if (t.kind === 'keyword' && /^TYPE$/i.test(t.text)) return { kind: 'TYPE', name: '' }
  }
  return null
}

// wordAt finds the name, with its dotted parts such as t.Q, at an offset of the source.
export function wordAt(src: string, offset: number): string | null {
  for (const t of tokenize(src)) {
    if (offset >= t.start && offset <= t.start + t.text.length && t.kind === 'ident') {
      let start = t.start
      let end = t.start + t.text.length
      while (start > 1 && src[start - 1] === '.' && /[A-Za-z0-9_]/.test(src[start - 2])) {
        start -= 2
        while (start > 0 && /[A-Za-z0-9_]/.test(src[start - 1])) start--
      }
      while (src[end] === '.' && /[A-Za-z_]/.test(src[end + 1] ?? '')) {
        end++
        while (end < src.length && /[A-Za-z0-9_]/.test(src[end])) end++
      }
      return src.slice(start, end)
    }
  }
  return null
}

// Monitored is a variable as the editor shows it online.
export interface Monitored {
  path: string
  value: string
  type: string
  forced?: boolean
  writable: boolean
}
