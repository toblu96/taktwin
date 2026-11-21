// A small part of Markdown, enough for an exercise's instructions: headings, paragraphs,
// lists, **bold**, *italics* and `code`. It gives blocks for Vue to show as text, so nothing
// in a file handed out can run in the app.

export type Span = { text: string; bold?: boolean; em?: boolean; code?: boolean }
export type Block = { kind: 'heading'; level: number; spans: Span[] } | { kind: 'paragraph'; spans: Span[] } | { kind: 'list'; ordered: boolean; items: Span[][] }

export function spans(text: string): Span[] {
  const out: Span[] = []
  const pattern = /(`[^`]+`|\*\*[^*]+\*\*|\*[^*]+\*|_[^_]+_)/g
  let at = 0
  for (const m of text.matchAll(pattern)) {
    if (m.index > at) out.push({ text: text.slice(at, m.index) })
    const t = m[0]
    if (t.startsWith('`')) out.push({ text: t.slice(1, -1), code: true })
    else if (t.startsWith('**')) out.push({ text: t.slice(2, -2), bold: true })
    else out.push({ text: t.slice(1, -1), em: true })
    at = m.index + t.length
  }
  if (at < text.length) out.push({ text: text.slice(at) })
  return out
}

export function markdown(source: string): Block[] {
  const blocks: Block[] = []
  let paragraph: string[] = []
  let list: { ordered: boolean; items: Span[][] } | null = null
  const flush = () => {
    if (paragraph.length) blocks.push({ kind: 'paragraph', spans: spans(paragraph.join(' ')) })
    paragraph = []
    if (list) blocks.push({ kind: 'list', ...list })
    list = null
  }
  for (const raw of source.replace(/\r\n/g, '\n').split('\n')) {
    const line = raw.trim()
    const heading = /^(#{1,4})\s+(.*)$/.exec(line)
    const item = /^(?:([-*+])|(\d+)[.)])\s+(.*)$/.exec(line)
    if (!line) {
      flush()
    } else if (heading) {
      flush()
      blocks.push({ kind: 'heading', level: heading[1].length, spans: spans(heading[2]) })
    } else if (item) {
      const ordered = !!item[2]
      if (paragraph.length || (list && list.ordered !== ordered)) flush()
      list ??= { ordered, items: [] }
      list.items.push(spans(item[3]))
    } else if (list && raw.startsWith(' ')) {
      const last = list.items[list.items.length - 1]
      last.push({ text: ' ' }, ...spans(line))
    } else {
      if (list) flush()
      paragraph.push(line)
    }
  }
  flush()
  return blocks
}
