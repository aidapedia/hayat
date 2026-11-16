import { useMemo } from 'react'
import { CatalogService, RegistryService } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { servicesOf, type Catalog } from '../lib/roles'

/**
 * Everything the roles pages read, loaded together: the roles, the catalog
 * (node types and permissions) and the services that published them.
 */
export function useRolesData() {
  const catalog = useClient(CatalogService)
  const registry = useClient(RegistryService)
  const q = useQuery(async () => {
    const [roles, cat, services] = await Promise.all([
      catalog.listRoles({}),
      catalog.getCatalog({}),
      // Only names and descriptions come from here; a page still works without them.
      registry.listServices({}).catch(() => ({ services: [] })),
    ])
    const data: Catalog = { types: cat.types, permissions: cat.permissions, services: services.services }
    return { roles: roles.roles, catalog: data }
  }, [catalog, registry])

  const services = useMemo(() => (q.data ? servicesOf(q.data.catalog) : []), [q.data])
  return { ...q, roles: q.data?.roles ?? [], catalog: q.data?.catalog, services }
}
