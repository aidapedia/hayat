import { useState } from 'react'
import { useNavigate } from 'react-router'
import { Building2, Plus } from 'lucide-react'
import { TenantService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { useClient } from '@/shared/api/client'
import { useServerPagination } from '@/shared/ui/table/usePagination'
import { formatDate } from '@/shared/lib/datetime'
import { useMe } from '@/features/auth'
import { EmptyState, ErrorState, LoadingState, LogoTile, SearchInput } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Pagination } from '@/shared/ui/table/Pagination'
import { CreateTenantDialog } from './components/CreateTenantDialog'

export default function TenantsPage() {
  const { isAdmin } = useMe()
  const tenants = useClient(TenantService)
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [creating, setCreating] = useState(false)

  const list = useServerPagination(
    async (token, pageSize) => {
      const res = await tenants.listTenants({ query, pageSize, pageToken: token })
      return { items: res.tenants, next: res.nextPageToken }
    },
    [tenants, query],
  )

  return (
    <>
      <PageHeader
        title="Tenants"
        desc={
          isAdmin
            ? 'Organisations shared by every service. A tabletsgo workspace, a billing account and a payments merchant can all belong to one tenant.'
            : 'The organisations you belong to. Open one to see its members and its Resource Tree.'
        }
        action={
          isAdmin && (
            <Button size="lg" onClick={() => setCreating(true)}>
              <Plus /> Create tenant
            </Button>
          )
        }
      />
      <div className="mt-7 flex flex-col gap-4">
        <SearchInput placeholder="Search by name or slug" value={query} onChange={(e) => setQuery(e.target.value)} className="max-w-sm" />
        {list.error ? (
          <ErrorState message={list.error} onRetry={list.reload} />
        ) : list.loading && list.items.length === 0 ? (
          <LoadingState />
        ) : list.items.length === 0 ? (
          <EmptyState icon={Building2} title={query ? 'No tenant matches that search' : 'No tenants yet'}>
            {!query && (isAdmin ? 'Create the first one to group people and the resources they share.' : "You don't belong to any tenant yet.")}
          </EmptyState>
        ) : (
          <div className="overflow-hidden rounded-xl border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="text-[11px]">Tenant</TableHead>
                  <TableHead className="text-[11px]">Members</TableHead>
                  <TableHead className="text-[11px]">Region</TableHead>
                  <TableHead className="text-[11px]">Created</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.items.map((t) => (
                  <TableRow key={t.id} className="cursor-pointer" onClick={() => navigate(`/tenants/${t.id}`)}>
                    <TableCell>
                      <div className="flex items-center gap-2.5">
                        <LogoTile name={t.name} src={t.logoUrl} size={30} />
                        <div className="min-w-0">
                          <div className="text-[13px] font-semibold">{t.name}</div>
                          <div className="font-mono text-[12px] text-faint">{t.slug}</div>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell className="text-[13px] tabular-nums">{t.memberCount}</TableCell>
                    <TableCell className="text-[12px] text-muted-foreground">{t.region}</TableCell>
                    <TableCell className="text-[12px] text-muted-foreground">{formatDate(t.createdAt)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <Pagination {...list.pager} className="border-t" />
          </div>
        )}
      </div>
      {creating && <CreateTenantDialog onClose={() => setCreating(false)} onCreated={(t) => navigate(`/tenants/${t.id}`)} />}
    </>
  )
}
