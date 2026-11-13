import type { Client } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import type { Contract } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import type { Manifest, Summary } from './contract/manifest'

/** The built-in service: Hayat itself, where the console's client lives. */
export const PLATFORM = 'hayat'

/** A sign-in client's kind, the way people name it. */
export function clientKind(c: Pick<Client, 'kind'>): { label: string; detail: string } {
  return c.kind === 'public' ? { label: 'Public', detail: 'Browser, mobile or CLI · PKCE' } : { label: 'Confidential', detail: 'Server · client secret' }
}

/** Where a client sends people back to, shortened for a list. */
export function returnsTo(c: Pick<Client, 'redirectUris'>): string {
  const first = c.redirectUris[0]
  if (!first) return '—'
  try {
    const u = new URL(first)
    return u.host + u.pathname
  } catch {
    return first
  }
}

export function readManifest(json: string | undefined): Manifest | null {
  if (!json) return null
  try {
    return JSON.parse(json) as Manifest
  } catch {
    return null
  }
}

export function readSummary(json: string | undefined): Summary {
  try {
    return json ? (JSON.parse(json) as Summary) : {}
  } catch {
    return {}
  }
}

/** How many entries a summary adds. */
export function summaryCount(s: Summary): number {
  return (
    (s.added_types?.length ?? 0) +
    (s.added_permissions?.length ?? 0) +
    (s.added_roles?.length ?? 0) +
    Object.values(s.added_parents ?? {}).reduce((n, v) => n + v.length, 0) +
    Object.values(s.added_role_permissions ?? {}).reduce((n, v) => n + v.length, 0)
  )
}

/** The service's own version that waits for another namespace, if any. */
export const ownPending = (contracts: Contract[]) => contracts.find((c) => c.status === 'pending')
export const activeContract = (contracts: Contract[]) => contracts.find((c) => c.status === 'active')
