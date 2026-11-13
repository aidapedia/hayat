import { useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { CatalogService, type Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { ErrorState, Field, LoadingState } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Input } from '@/shared/ui/input'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import { Textarea } from '@/shared/ui/textarea'
import { GrantablePicker } from './components/editor/GrantablePicker'
import { PermissionPicker } from './components/editor/PermissionPicker'
import { RoleSummary } from './components/editor/RoleSummary'
import { ServiceRoleNotice } from './components/editor/ServiceRoleNotice'
import { StartFrom } from './components/editor/StartFrom'
import { useRolesData } from './hooks/use-roles-data'
import { slugify, sourceOf, type Catalog, type ServiceMeta } from './lib/roles'

type Draft = { name: string; description: string; permissions: string[]; appliesTo: string[] }

/**
 * Creating a role (/roles/new, ?from=<id> to start from a copy) and editing one
 * (/roles/:id/edit). A whole page, so the permissions have room and the live
 * summary can sit beside them. A service's role can't be edited and says why.
 */
export default function RoleEditorPage() {
  const { id } = useParams()
  const [params] = useSearchParams()
  const { roles, catalog, services, error, loading, reload } = useRolesData()
  const editing = id ? roles.find((r) => r.id === id) : undefined
  const from = params.get('from') ? roles.find((r) => r.id === params.get('from')) : undefined

  const title = id ? `Edit ${editing?.name ?? 'role'}` : 'Create a role'
  const header = <PageHeader title={title} crumbs={[{ label: id ? (editing?.name ?? 'Role') : 'New role' }]} />

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
  if (id && !editing)
    return (
      <>
        {header}
        <div className="mt-6">
          <ErrorState message="This role doesn't exist any more." />
        </div>
      </>
    )
  if (editing && sourceOf(editing) === 'service')
    return (
      <>
        {header}
        <ServiceRoleNotice role={editing} catalog={catalog} services={services} />
      </>
    )

  return (
    <>
      {header}
      {/* Keyed so a different role, or a different starting copy, starts a fresh form. */}
      <RoleForm key={editing?.id ?? from?.id ?? 'new'} role={editing} from={from} roles={roles} catalog={catalog} services={services} />
    </>
  )
}

function draftOf(role: Role | undefined, from: Role | undefined): Draft {
  if (role) return { name: role.name, description: role.description, permissions: [...role.permissions], appliesTo: [...role.appliesTo] }
  if (from) return { name: `${from.name} (copy)`, description: from.description, permissions: [...from.permissions], appliesTo: [...from.appliesTo] }
  return { name: '', description: '', permissions: [], appliesTo: [] }
}

function RoleForm({ role, from, roles, catalog, services }: { role?: Role; from?: Role; roles: Role[]; catalog: Catalog; services: ServiceMeta[] }) {
  const api = useClient(CatalogService)
  const navigate = useNavigate()
  const [d, setD] = useState<Draft>(() => draftOf(role, from))
  const [template, setTemplate] = useState(from?.id ?? '')
  const [saving, setSaving] = useState(false)

  const perms = catalog.permissions.filter((p) => d.permissions.includes(p.key))
  const name = d.name.trim()
  const canSave = name.length > 0 && name.length <= 80 && d.permissions.length > 0
  const saveLabel = role ? 'Save changes' : 'Create role'

  const save = async () => {
    setSaving(true)
    try {
      const input = { name, description: d.description.trim(), appliesTo: d.appliesTo, permissions: d.permissions }
      const saved = role ? await api.updateRole({ id: role.id, ...input }) : await api.createRole(input)
      toast.success(role ? 'Role saved' : 'Role created')
      navigate(`/roles?role=${saved.role?.id ?? ''}`)
    } catch (err) {
      toast.error(errorMessage(err))
      setSaving(false)
    }
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        if (canSave) void save()
      }}
      className="mt-6 flex flex-wrap items-start gap-6"
    >
      <div className="flex min-w-0 flex-[999_1_560px] flex-col gap-[18px]">
        {!role && (
          <StartFrom
            roles={roles}
            services={services}
            value={template}
            onPick={(r) => {
              setTemplate(r?.id ?? '')
              // Keep what the admin typed; take the copy's permissions and reach.
              setD({ ...d, name: d.name || (r ? `${r.name} (copy)` : ''), permissions: r ? [...r.permissions] : [], appliesTo: r ? [...r.appliesTo] : [] })
            }}
          />
        )}

        <section aria-labelledby="basics-title" className="flex flex-col gap-3.5 rounded-2xl border bg-background px-5 py-[18px]">
          <h2 id="basics-title" className="text-[14px] font-semibold">
            About this role
          </h2>
          <div className="grid grid-cols-2 gap-3.5 max-[720px]:grid-cols-1">
            <Field
              label="Name"
              htmlFor="role-name"
              hint={
                role ? (
                  <>
                    Identifier <span className="font-mono text-muted-foreground">{role.slug}</span>. It stays the same when you rename the role.
                  </>
                ) : (
                  <>
                    Identifier <span className="font-mono text-muted-foreground">{slugify(name)}</span>, which services and the API see.
                  </>
                )
              }
            >
              <Input id="role-name" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="e.g. Database admin" maxLength={80} required autoFocus={!role} />
            </Field>
            <Field label="What is it for?" htmlFor="role-desc">
              <Textarea
                id="role-desc"
                value={d.description}
                onChange={(e) => setD({ ...d, description: e.target.value })}
                rows={3}
                placeholder="One sentence a tenant admin reads when they grant it."
              />
            </Field>
          </div>
        </section>

        <PermissionPicker catalog={catalog} services={services} value={d.permissions} appliesTo={d.appliesTo} onChange={(permissions) => setD({ ...d, permissions })} />
        <GrantablePicker types={catalog.types} services={services} perms={perms} value={d.appliesTo} onChange={(appliesTo) => setD({ ...d, appliesTo })} />

        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" asChild>
            <Link to={role ? `/roles?role=${role.id}` : '/roles'}>Cancel</Link>
          </Button>
          <Button type="submit" disabled={!canSave || saving}>
            {saving ? 'Saving…' : saveLabel}
          </Button>
        </div>
      </div>

      <aside aria-label="Role summary" className="flex min-w-0 flex-[1_1_320px] flex-col gap-3 lg:sticky lg:top-4">
        <RoleSummary
          name={d.name}
          catalog={catalog}
          services={services}
          perms={perms}
          appliesTo={d.appliesTo}
          original={role?.permissions}
          grantCount={role?.grantCount ?? 0}
          saving={saving}
          canSave={canSave}
          saveLabel={saveLabel}
          onAllow={(type) => setD({ ...d, appliesTo: [...new Set([...d.appliesTo, type])] })}
          onSave={() => void save()}
        />
        {!role && <p className="px-1 text-[12px] text-muted-foreground">After you create it, grant it from the Resource Tree: pick a node, then grant access.</p>}
      </aside>
    </form>
  )
}
