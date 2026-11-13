/**
 * Just enough YAML for a contract manifest, so the form and the YAML tab can
 * trade places without a dependency: block maps and lists, flow `[a, b]` and
 * `{ k: v }`, quoted and plain scalars, comments. Anchors, tags and `|`/`>`
 * blocks are out; `parseYaml` throws on them, and the YAML tab still publishes
 * the text as written (the server's parser is the real one).
 */

type Line = { indent: number; text: string; no: number }

export class YamlError extends Error {}

function stripComment(s: string): string {
  let quote = ''
  for (let i = 0; i < s.length; i++) {
    const c = s[i]
    if (quote) {
      if (c === '\\' && quote === '"') i++
      else if (c === quote) quote = ''
    } else if (c === '"' || c === "'") quote = c
    else if (c === '#' && (i === 0 || /\s/.test(s[i - 1]))) return s.slice(0, i)
  }
  return s
}

function toLines(src: string): Line[] {
  const out: Line[] = []
  src.split(/\r?\n/).forEach((raw, i) => {
    if (/^\s*\t/.test(raw)) throw new YamlError(`line ${i + 1}: indent with spaces, not tabs`)
    const text = stripComment(raw).trimEnd()
    if (!text.trim() || text.trim() === '---') return
    out.push({ indent: text.length - text.trimStart().length, text: text.trimStart(), no: i + 1 })
  })
  return out
}

const isItem = (t: string) => t === '-' || t.startsWith('- ')

/** Index of the `:` that ends a mapping key, or -1. */
function keyColon(t: string): number {
  if (t[0] === '[' || t[0] === '{') return -1
  let i = 0
  if (t[0] === '"' || t[0] === "'") {
    const q = t[0]
    for (i = 1; i < t.length && t[i] !== q; i++) if (t[i] === '\\' && q === '"') i++
    i++
  }
  for (; i < t.length; i++) if (t[i] === ':' && (i + 1 === t.length || t[i + 1] === ' ')) return i
  return -1
}

function scalar(s: string): unknown {
  const t = s.trim()
  if (t === '' || t === '~' || t === 'null') return null
  if (t === 'true') return true
  if (t === 'false') return false
  if (/^-?\d+$/.test(t)) return Number(t)
  if (/^[&*!|>%@`]/.test(t)) throw new YamlError(`"${t}" uses YAML this editor doesn't read`)
  return t
}

/** A flow value (`[..]`, `{..}`, quoted or plain) starting at pos. */
function flow(s: string, pos: number, inFlow: boolean): [unknown, number] {
  const ws = () => {
    while (pos < s.length && s[pos] === ' ') pos++
  }
  ws()
  const c = s[pos]
  if (c === '[') {
    const list: unknown[] = []
    pos++
    ws()
    if (s[pos] === ']') return [list, pos + 1]
    for (;;) {
      const [v, p] = flow(s, pos, true)
      list.push(v)
      pos = p
      ws()
      if (s[pos] === ',') pos++
      else if (s[pos] === ']') return [list, pos + 1]
      else throw new YamlError(`expected , or ] in "${s}"`)
    }
  }
  if (c === '{') {
    const map: Record<string, unknown> = {}
    pos++
    ws()
    if (s[pos] === '}') return [map, pos + 1]
    for (;;) {
      const [k, p] = flow(s, pos, true)
      pos = p
      ws()
      if (s[pos] !== ':') throw new YamlError(`expected : after "${String(k)}" in "${s}"`)
      const [v, p2] = flow(s, pos + 1, true)
      map[String(k)] = v
      pos = p2
      ws()
      if (s[pos] === ',') pos++
      else if (s[pos] === '}') return [map, pos + 1]
      else throw new YamlError(`expected , or } in "${s}"`)
    }
  }
  if (c === '"') {
    let end = pos + 1
    while (end < s.length && s[end] !== '"') end += s[end] === '\\' ? 2 : 1
    if (end >= s.length) throw new YamlError(`unclosed quote in "${s}"`)
    return [JSON.parse(s.slice(pos, end + 1)), end + 1]
  }
  if (c === "'") {
    let out = ''
    let i = pos + 1
    for (;;) {
      if (i >= s.length) throw new YamlError(`unclosed quote in "${s}"`)
      if (s[i] === "'") {
        if (s[i + 1] === "'") {
          out += "'"
          i += 2
          continue
        }
        return [out, i + 1]
      }
      out += s[i++]
    }
  }
  if (!inFlow) return [scalar(s.slice(pos)), s.length]
  let end = pos
  while (end < s.length && !',]}'.includes(s[end]) && !(s[end] === ':' && (s[end + 1] === ' ' || ',]}'.includes(s[end + 1] ?? '')))) end++
  return [scalar(s.slice(pos, end)), end]
}

function inline(t: string, no: number): unknown {
  try {
    const [v, end] = flow(t, 0, false)
    if (t.slice(end).trim()) throw new YamlError(`unexpected "${t.slice(end).trim()}"`)
    return v
  } catch (e) {
    throw new YamlError(`line ${no}: ${(e as Error).message}`)
  }
}

function block(ls: Line[], i: number, indent: number): [unknown, number] {
  return isItem(ls[i].text) ? seq(ls, i, indent) : map(ls, i, indent)
}

function seq(ls: Line[], i: number, indent: number): [unknown[], number] {
  const out: unknown[] = []
  while (i < ls.length && ls[i].indent === indent && isItem(ls[i].text)) {
    const { text, no } = ls[i]
    const rest = text.slice(1).trimStart()
    if (!rest) {
      if (i + 1 < ls.length && ls[i + 1].indent > indent) {
        const [v, n] = block(ls, i + 1, ls[i + 1].indent)
        out.push(v)
        i = n
      } else {
        out.push(null)
        i++
      }
    } else if (keyColon(rest) > 0 || isItem(rest)) {
      // "- key: value" opens a map whose keys line up with "key".
      const col = indent + text.length - rest.length
      ls[i] = { indent: col, text: rest, no }
      const [v, n] = block(ls, i, col)
      out.push(v)
      i = n
    } else {
      out.push(inline(rest, no))
      i++
    }
  }
  return [out, i]
}

function map(ls: Line[], i: number, indent: number): [Record<string, unknown>, number] {
  const out: Record<string, unknown> = {}
  while (i < ls.length && ls[i].indent === indent && !isItem(ls[i].text)) {
    const { text, no } = ls[i]
    const at = keyColon(text)
    if (at <= 0) throw new YamlError(`line ${no}: expected "key: value"`)
    const key = String(inline(text.slice(0, at), no))
    const rest = text.slice(at + 1).trim()
    i++
    if (rest) out[key] = inline(rest, no)
    else if (i < ls.length && (ls[i].indent > indent || (ls[i].indent === indent && isItem(ls[i].text)))) {
      const [v, n] = block(ls, i, ls[i].indent)
      out[key] = v
      i = n
    } else out[key] = null
  }
  return [out, i]
}

export function parseYaml(src: string): unknown {
  const ls = toLines(src)
  if (ls.length === 0) return null
  const [v, i] = block(ls, 0, ls[0].indent)
  if (i < ls.length) throw new YamlError(`line ${ls[i].no}: indentation doesn't line up`)
  return v
}

/** A string as a YAML scalar, quoted only when plain would read differently. */
export function yamlString(s: string): string {
  const plain =
    s !== '' &&
    s === s.trim() &&
    !/^[-?:,[\]{}#&*!|>'"%@`]/.test(s) &&
    !/: |:$| #|[,[\]{}]|\n/.test(s) &&
    !/^(true|false|null|~|-?\d+(\.\d+)?)$/i.test(s)
  return plain ? s : JSON.stringify(s)
}
