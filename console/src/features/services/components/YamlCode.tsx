import { useRef, type ReactNode } from 'react'
import { cn } from '@/shared/lib/utils'

// A small YAML highlighter for contract manifests: keys, strings, numbers and
// booleans, comments, and the punctuation between them. It reads a line at a
// time, which is all a manifest needs; anything it doesn't recognise is plain.

const KEY = 'text-code-key'
const STRING = 'text-code-string'
const SCALAR = 'text-code-scalar'
const COMMENT = 'text-faint italic'
const PUNCT = 'text-muted-foreground'

// One token per alternative: a quoted string, an inline comment, flow
// punctuation, a `key:` inside a flow map, a boolean/null, a number.
const VALUE_RE =
  /("(?:[^"\\]|\\.)*"|'(?:[^']|'')*')|(\s#.*$)|([[\]{},])|([A-Za-z0-9_.\-/]+)(\s*:)(?=\s|$)|\b(true|false|null|yes|no|~)\b|(-?\d+(?:\.\d+)?)(?=[\s,\]}]|$)/g

function value(text: string, key: string): ReactNode[] {
  const out: ReactNode[] = []
  let last = 0
  let i = 0
  for (const m of text.matchAll(VALUE_RE)) {
    const at = m.index ?? 0
    if (at > last) out.push(text.slice(last, at))
    const k = `${key}-${i++}`
    if (m[1]) out.push(<span key={k} className={STRING}>{m[1]}</span>)
    else if (m[2]) out.push(<span key={k} className={COMMENT}>{m[2]}</span>)
    else if (m[3]) out.push(<span key={k} className={PUNCT}>{m[3]}</span>)
    else if (m[4])
      out.push(
        <span key={k}>
          <span className={KEY}>{m[4]}</span>
          <span className={PUNCT}>{m[5]}</span>
        </span>,
      )
    else if (m[6]) out.push(<span key={k} className={SCALAR}>{m[6]}</span>)
    else if (m[7]) out.push(<span key={k} className={SCALAR}>{m[7]}</span>)
    last = at + m[0].length
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}

function line(text: string, n: number): ReactNode {
  const indent = /^\s*/.exec(text)![0]
  let rest = text.slice(indent.length)
  if (rest.startsWith('#')) return [indent, <span key="c" className={COMMENT}>{rest}</span>]
  const parts: ReactNode[] = [indent]
  // List markers: "- ", possibly nested ("- - ").
  let dashes = ''
  while (/^-(\s|$)/.test(rest)) {
    const d = /^-\s*/.exec(rest)![0]
    dashes += d
    rest = rest.slice(d.length)
  }
  if (dashes) parts.push(<span key="d" className={PUNCT}>{dashes}</span>)
  const k = /^([A-Za-z0-9_.\-/]+)(\s*:)(?=\s|$)/.exec(rest)
  if (k) {
    parts.push(<span key="k" className={KEY}>{k[1]}</span>, <span key="p" className={PUNCT}>{k[2]}</span>)
    rest = rest.slice(k[0].length)
  }
  parts.push(...value(rest, `v${n}`))
  return parts
}

/** YAML with its parts coloured. */
export function YamlHighlight({ text }: { text: string }) {
  const lines = text.split('\n')
  return (
    <>
      {lines.map((l, i) => (
        <span key={i}>
          {line(l, i)}
          {i < lines.length - 1 && '\n'}
        </span>
      ))}
    </>
  )
}

/** A read-only block of highlighted YAML. */
export function YamlBlock({ text, className }: { text: string; className?: string }) {
  return (
    <pre className={cn('overflow-auto bg-muted/40 p-4 font-mono text-[12px] leading-relaxed', className)}>
      <YamlHighlight text={text} />
    </pre>
  )
}

const LAYER = 'm-0 whitespace-pre-wrap break-words p-3 font-mono text-[12px] leading-relaxed'

/**
 * A YAML editor: a plain textarea whose text is transparent, over the same
 * text highlighted. Both use the same font, padding and wrapping, and the
 * highlight scrolls with the textarea, so the coloured text sits exactly under
 * what is typed.
 */
export function YamlEditor({ value, onChange, rows = 24, ariaLabel }: { value: string; onChange: (v: string) => void; rows?: number; ariaLabel: string }) {
  const under = useRef<HTMLPreElement>(null)
  return (
    <div className="relative overflow-hidden rounded-md border bg-muted/30 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50">
      <pre ref={under} aria-hidden="true" className={cn(LAYER, 'pointer-events-none absolute inset-0 overflow-hidden')}>
        {/* A trailing newline keeps the last line's height when it is empty. */}
        <YamlHighlight text={value + '\n'} />
      </pre>
      {/* A raw textarea: the shared Textarea can't be made transparent over a highlight layer. */}
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onScroll={(e) => {
          if (under.current) under.current.scrollTop = e.currentTarget.scrollTop
        }}
        rows={rows}
        spellCheck={false}
        aria-label={ariaLabel}
        className={cn(LAYER, 'relative block w-full resize-y bg-transparent text-transparent caret-foreground outline-none selection:bg-primary/25')}
      />
    </div>
  )
}
