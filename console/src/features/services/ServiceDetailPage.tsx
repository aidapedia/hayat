import { useState } from 'react'
import { Navigate, useParams } from 'react-router'
import { MoreHorizontal, Pencil } from 'lucide-react'
import { RegistryService } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { ErrorState, LoadingState, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/shared/ui/dropdown-menu'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import { PageTabs, type PageTab } from '@/shared/ui/page/PageTabs'
import type { Crumb } from '@/shared/ui/page/PageTrail'
import { EditServiceDialog } from './components/EditServiceDialog'
import { ServiceTile } from './components/ServiceTile'
import { ownPending } from './lib'
import { ClientDetail } from './tabs/ClientDetail'
import { ClientsTab } from './tabs/ClientsTab'
import { ContractTab } from './tabs/ContractTab'
import { OverviewTab } from './tabs/OverviewTab'
import { OwnersTab } from './tabs/OwnersTab'
import { PublishContract } from './tabs/PublishContract'

/**
 * One service: its header, then a tab per part (each an address):
 * /services/:id, /contract, /contract/publish, /clients, /clients/:clientId, /owners.
 * Hayat's own service has only an overview and its clients.
 */
export default function ServiceDetailPage() {
  const { id = '', tab = '', sub = '' } = useParams()
  const registry = useClient(RegistryService)
  const { data, error, loading, reload } = useQuery(() => registry.getService({ id }), [registry, id])
  const [editing, setEditing] = useState(false)

  if (error) return <ErrorState message={error} onRetry={reload} />
  if (loading && !data) return <LoadingState />
  if (!data?.service) return null
  const s = data.service
  const base = `/services/${s.id}`
  const pendingWaits = data.incoming.length + (ownPending(data.contracts) ? 1 : 0)

  const tabs: PageTab[] = s.builtin
    ? [
        { to: base, label: 'Overview', end: true },
        { to: `${base}/clients`, label: `Clients · ${s.signInClients}` },
      ]
    : [
        { to: base, label: 'Overview', end: true },
        { to: `${base}/contract`, label: pendingWaits > 0 ? `Contract · ${pendingWaits} pending` : 'Contract' },
        { to: `${base}/clients`, label: `Clients · ${s.signInClients}` },
        { to: `${base}/owners`, label: `Owners · ${s.owners.length}` },
      ]
  const known = s.builtin ? ['', 'clients'] : ['', 'contract', 'clients', 'owners']
  if (!known.includes(tab)) return <Navigate to={base} replace />

  const crumbs: Crumb[] = [{ label: s.displayName, to: tab ? base : undefined }]
  if (tab === 'contract' && sub === 'publish') crumbs.push({ label: 'Contract', to: `${base}/contract` }, { label: 'Publish a version' })

  let body
  switch (tab) {
    case 'contract':
      body = sub === 'publish' ? <PublishContract data={data} onPublished={reload} /> : <ContractTab data={data} onChanged={reload} />
      break
    case 'clients':
      body = sub ? <ClientDetail service={s} clientId={sub} /> : <ClientsTab service={s} canManage={data.canManage} onChanged={reload} />
      break
    case 'owners':
      body = <OwnersTab service={s} canManage={data.canManage} onChanged={reload} />
      break
    default:
      body = <OverviewTab data={data} onChanged={reload} />
  }

  return (
    <>
      <PageHeader
        title={s.displayName}
        crumbs={crumbs}
        media={<ServiceTile name={s.displayName} builtin={s.builtin} size={44} />}
        meta={
          <>
            <Pill className="font-mono normal-case">{s.id}</Pill>
            {s.builtin ? <Pill>Built-in</Pill> : s.activeVersion > 0 && <Pill className="border-success/30 text-success">v{s.activeVersion} active</Pill>}
          </>
        }
        desc={s.description || undefined}
        action={
          data.canManage &&
          !s.builtin && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="icon-lg" aria-label={`More actions for ${s.displayName}`}>
                  <MoreHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-44">
                <DropdownMenuItem onSelect={() => setEditing(true)}>
                  <Pencil /> Edit service
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )
        }
      />
      <PageTabs tabs={tabs} />
      <div className="mt-7">{body}</div>
      {editing && <EditServiceDialog service={s} onClose={() => setEditing(false)} onSaved={reload} />}
    </>
  )
}
