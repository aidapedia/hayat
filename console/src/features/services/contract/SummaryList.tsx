import type { Summary } from './manifest'

/** What a contract version adds to the catalog, one line per kind of change. */
export function SummaryList({ summary: s, empty = 'No changes to the catalog.' }: { summary: Summary; empty?: string }) {
  const lines: [string, string[]][] = [
    ['Adds types', s.added_types ?? []],
    ['Adds permissions', s.added_permissions ?? []],
    ['Adds roles', s.added_roles ?? []],
    ...Object.entries(s.added_parents ?? {}).map(([t, ps]): [string, string[]] => [`New parents for ${t}`, ps]),
    ...Object.entries(s.added_role_permissions ?? {}).map(([r, ps]): [string, string[]] => [`More permissions in ${r}`, ps]),
  ].filter(([, v]) => v.length > 0) as [string, string[]][]
  if (lines.length === 0) return <p className="text-[12px] text-faint">{empty}</p>
  return (
    <ul className="flex flex-col gap-1">
      {lines.map(([label, items]) => (
        <li key={label} className="text-[12px]">
          <span className="text-muted-foreground">{label}: </span>
          <span className="font-mono">{items.join(', ')}</span>
        </li>
      ))}
    </ul>
  )
}

/** The same, from a stored contract's summary JSON. */
export function SummaryJson({ json }: { json: string }) {
  let s: Summary = {}
  try {
    s = JSON.parse(json)
  } catch {
    /* shown as nothing */
  }
  return <SummaryList summary={s} />
}
