import type { Client } from '@connectrpc/connect'
import { TenantService, type Member, type Tenant } from '@/shared/api/gen/hayat/admin/v1/admin_pb'

/**
 * The paged admin lists, read whole. The grant and group pickers choose from
 * everything, not from the first page — a roster or a tenant list longer than
 * one page would otherwise hide the person you meant.
 */
async function all<T>(page: (token: string) => Promise<{ items: T[]; next: string }>) {
  const out: T[] = []
  let token = ''
  do {
    const res = await page(token)
    out.push(...res.items)
    token = res.next
  } while (token)
  return out
}

export const allMembers = (api: Client<typeof TenantService>, tenantId: string): Promise<Member[]> =>
  all((pageToken) => api.listMembers({ tenantId, pageSize: 200, pageToken }).then((r) => ({ items: r.members, next: r.nextPageToken })))

/** Every tenant the caller may see: all of them for an instance admin, their own otherwise. */
export const allTenants = (api: Client<typeof TenantService>): Promise<Tenant[]> =>
  all((pageToken) => api.listTenants({ pageSize: 200, pageToken }).then((r) => ({ items: r.tenants, next: r.nextPageToken })))
