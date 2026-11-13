import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { Check, Copy, Info, Pencil, ShieldPlus, Trash2 } from 'lucide-react'
import type { Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { GrantRoleDialog } from '@/features/resource-tree'
import { CopyField, DetailList, ErrorState, LoadingState } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { PageHeader, Section } from '@/shared/ui/page/PageHeader'
import { DeleteRoleDialog } from './components/DeleteRoleDialog'
import { RoleIcon, RoleSourceBadge, ServiceMark } from './components/marks'
import { RoleGrantsList, usageLine, useRoleGrants } from './components/RoleGrants'
import { TypeTree } from './components/TypeTree'
import { useRolesData } from './hooks/use-roles-data'
import { grantableLine, groupByService, permLabel, plural, sourceOf, type Catalog, type ServiceMeta } from './lib/roles'

/** Why a role can or can't be changed, said once, where the buttons are. */
function noteFor(role: Role, services: ServiceMeta[]): string {
  switch (sourceOf(role)) {
    case 'builtin':
      return 'Built-in roles can be tuned but not deleted. A change applies to every grant of this role at once.'
    case 'service': {
      const name = services.find((s) => s.id === role.service)?.name ?? role.service
      return `${name}'s contract defines this role, so it changes only with a new contract version. Duplicate it to make your own variation.`
    }
    default:
      return role.grantCount > 0 ? `Granted ${plural(role.grantCount, 'time')}. Remove those grants before you can delete it.` : ''
  }
}

/**
 * One role in full (/roles/:id): what it lets someone do, where it can be
 * granted, and who holds it. A whole page, so every grant has room and a link
 * to it lands here — it used to be a drawer over the list.
 */
export default function RoleDetailPage() {
  const { id = '' } = useParams()
  const { roles, catalog, services, error, loading, reload } = useRolesData()
  const role = roles.find((r) => r.id === id)

  const header = <PageHeader title={role?.name ?? 'Role'} crumbs={[{ label: role?.name ?? 'Role' }]} />

  if (error)
    return (
      <>
        {header}
        <div className="mt-6">
          <ErrorState message={error} onRetry={reload} />
        </div>
      </>
    )
  if (loading && !catalog)
    return (
      <>
        {header}
        <LoadingState />
      </>
    )
  if (!catalog) return null
  if (!role)
    return (
      <>
        {header}
        <div className="mt-6">
          <ErrorState message="This role doesn't exist any more." />
        </div>
      </>
    )

  return <RoleBody key={role.id} role={role} catalog={catalog} services={services} onChange={reload} />
}

function RoleBody({ role, catalog, services, onChange }: { role: Role; catalog: Catalog; services: ServiceMeta[]; onChange: () => Promise<void> }) {
  const navigate = useNavigate()
  const [deleting, setDeleting] = useState(false)
  const [granting, setGranting] = useState(false)
  const src = sourceOf(role)
  const groups = groupByService(role.permissions, catalog, services)
  const note = noteFor(role, services)
  const { data, error, loading, reload } = useRoleGrants(role.id, role.grantCount)

  return (
    <>
      <PageHeader
        title={role.name}
        desc={role.description || undefined}
        crumbs={[{ label: role.name }]}
        media={<RoleIcon role={role} services={services} />}
        meta={
          <>
            <RoleSourceBadge role={role} services={services} />
            <span className="font-mono text-[11.5px] text-muted-foreground">{role.slug}</span>
          </>
        }
        action={
          <>
            {src !== 'service' && (
              <Button size="lg" asChild>
                <Link to={`/roles/${role.id}/edit`}>
                  <Pencil /> Edit role
                </Link>
              </Button>
            )}
            <Button size="lg" variant="outline" asChild>
              <Link to={`/roles/new?from=${role.id}`}>
                <Copy /> Duplicate
              </Link>
            </Button>
            {src === 'custom' && (
              <Button size="lg" variant="outline" onClick={() => setDeleting(true)} className="border-destructive/30 text-destructive hover:bg-destructive/5 hover:text-destructive">
                <Trash2 /> Delete
              </Button>
            )}
          </>
        }
      />

      {note && (
        <p className="mt-5 flex items-start gap-2 rounded-[10px] bg-muted/70 px-3 py-2.5 text-[12.5px] text-muted-foreground">
          <Info className="mt-0.5 size-3.5 shrink-0" />
          {note}
        </p>
      )}

      <div className="mt-7 grid grid-cols-[minmax(0,1fr)_minmax(280px,360px)] gap-x-10 max-[1000px]:grid-cols-1">
        <div className="min-w-0">
          <Section title="Someone with this role can" desc={groups.length === 0 ? undefined : 'Every permission this role bundles, by the service that defined it.'}>
            {groups.length === 0 ? (
              <p className="text-[13px] text-faint">Nothing yet: this role has no permissions.</p>
            ) : (
              <div className="flex flex-col gap-4 rounded-xl border p-4">
                {groups.map((g) => (
                  <div key={g.service.id} className="flex flex-col gap-2">
                    <div className="flex items-center gap-2">
                      <ServiceMark service={g.service} />
                      <span className="text-[13px] font-semibold">In {g.service.name}</span>
                      <span className="text-[12px] text-muted-foreground">
                        {g.perms.length} of {g.total}
                      </span>
                    </div>
                    <ul className="flex flex-col gap-1.5 pl-7">
                      {g.perms.map((p) => (
                        <li key={p.key} className="flex items-start gap-2">
                          <Check className="mt-0.5 size-3.5 shrink-0 text-primary" strokeWidth={2.6} />
                          <span className="flex flex-col">
                            <span className="text-[13px] font-medium">{permLabel(p)}</span>
                            {p.description && <span className="text-[12px] text-muted-foreground">{p.description}</span>}
                          </span>
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>
            )}
          </Section>

          <Section
            title={plural(role.grantCount, 'grant')}
            desc={data ? usageLine(data.grants, data.truncated) : role.grantCount === 0 ? 'Not granted anywhere yet' : undefined}
            action={
              <Button size="sm" variant="outline" onClick={() => setGranting(true)}>
                <ShieldPlus /> Grant this role
              </Button>
            }
          >
            {role.grantCount === 0 ? (
              <p className="text-[13px] text-faint">Nobody holds this role yet. Grant it to a person or group on a node of the resource tree.</p>
            ) : error ? (
              <ErrorState message={error} onRetry={reload} />
            ) : loading && !data ? (
              <LoadingState />
            ) : (
              <>
                <RoleGrantsList grants={data?.grants ?? []} />
                <p className="mt-2 text-[12px] text-muted-foreground">Each grant is managed from its node in the resource tree.</p>
              </>
            )}
          </Section>
        </div>

        <div className="min-w-0 max-[1000px]:mt-9">
          <Section title="Details">
            <div className="rounded-xl border p-4">
              <DetailList
                stacked
                items={[
                  ['Role id', <CopyField key="id" value={role.id} />],
                  ['Identifier', <CopyField key="slug" value={role.slug} />],
                  ['Owned by', src === 'builtin' ? 'Hayat (built-in)' : src === 'custom' ? 'This instance' : (services.find((s) => s.id === role.service)?.name ?? role.service)],
                  ['Permissions', plural(role.permissions.length, 'permission')],
                  ['Grantable on', grantableLine(role.appliesTo)],
                ]}
              />
            </div>
          </Section>
          <Section title="Where it can be granted" desc="The node types this role may be granted on.">
            <div className="rounded-xl border p-3">
              <TypeTree types={catalog.types} appliesTo={role.appliesTo} />
            </div>
          </Section>
        </div>
      </div>

      {granting && (
        <GrantRoleDialog
          role={role}
          onClose={() => setGranting(false)}
          onDone={async () => {
            setGranting(false)
            // The grant count lives on the role, so both it and the list re-read.
            await Promise.all([onChange(), reload()])
          }}
        />
      )}
      {deleting && <DeleteRoleDialog role={role} onClose={() => setDeleting(false)} onDeleted={() => navigate('/roles', { replace: true })} />}
    </>
  )
}
