import { timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt'

/**
 * Every time Hayat recorded is formatted here, in the time zone on the viewer's
 * account (falling back to the browser's), so all pages agree.
 */
let zone: string | undefined

export function setDisplayZone(z: string | undefined) {
  zone = z || undefined
}

const toDate = (t: Timestamp | Date | undefined) => (!t ? undefined : t instanceof Date ? t : timestampDate(t))

export function formatDateTime(t: Timestamp | Date | undefined): string {
  const d = toDate(t)
  if (!d) return '—'
  return d.toLocaleString(undefined, { timeZone: zone, dateStyle: 'medium', timeStyle: 'short' })
}

export function formatDate(t: Timestamp | Date | undefined): string {
  const d = toDate(t)
  if (!d) return '—'
  return d.toLocaleDateString(undefined, { timeZone: zone, dateStyle: 'medium' })
}

const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
]

/** "3 hours ago", "in 2 days". */
export function formatRelative(t: Timestamp | Date | undefined): string {
  const d = toDate(t)
  if (!d) return '—'
  const secs = (d.getTime() - Date.now()) / 1000
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
  for (const [unit, size] of units) {
    if (Math.abs(secs) >= size) return rtf.format(Math.round(secs / size), unit)
  }
  return 'just now'
}
