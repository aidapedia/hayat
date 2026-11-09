import { useMemo, useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { Pencil, Plus, ShieldHalf, Trash2 } from 'lucide-react'
import { CatalogService, type Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { EmptyState, ErrorState, Field, LoadingState, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Checkbox } from '@/shared/ui/checkbox'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { PageHeader, Section } from '@/shared/ui/page/PageHeader'
import { Textarea } from '@/shared/ui/textarea'

type Draft = { id?: string; name: string; description: string; appliesTo: string[]; permissions: string[] }

const toggle = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v])

function RoleDialog({ draft, onClose, onSaved }: { draft: Draft; onClose: () => void; onSaved: () => void }) {
  const catalog = useClient(CatalogService)
  const { data } = useQuery(() => catalog.getCatalog({}), [catalog])
  const [d, setD] = useState(draft)
  const [busy, setBusy] = useState(false)

  const byService = useMemo(() => {
    const groups = new Map<string, NonNullable<typeof data>['permissions']>()
    for (const p of data?.permissions ?? []) groups.set(p.service, [...(groups.get(p.service) ?? []), p])
    return [...groups.entries()]
  }, [data])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      if (d.id) await catalog.updateRole({ id: d.id, name: d.name, description: d.description, appliesTo: d.appliesTo, permissions: d.permissions })
      else await catalog.createRole({ name: d.name, description: d.description, appliesTo: d.appliesTo, permissions: d.permissions })
      toast.success(d.id ? 'Role saved' : 'Role created')
      onSaved()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">{d.id ? 'Edit role' : 'Create a role'}</DialogTitle>
            <DialogDescription className="text-[13px]">A role is a named set of permissions. It means nothing until it is granted on a node.</DialogDescription>
          </DialogHeader>
          <Field label="Name" htmlFor="role-name">
            <Input id="role-name" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} required autoFocus />
          </Field>
          <Field label="Description" htmlFor="role-desc">
            <Textarea id="role-desc" value={d.description} onChange={(e) => setD({ ...d, description: e.target.value })} rows={2} />
          </Field>
          <div className="flex flex-col gap-2">
            <span className="text-[12px] font-semibold text-muted-foreground">Can be granted on</span>
            <div className="flex flex-wrap gap-2">
              {(data?.types ?? []).map((t) => (
                <Label key={t.key} className="flex cursor-pointer items-center gap-2 rounded-[10px] border px-2.5 py-1.5 font-mono text-[12px] font-normal">
                  <Checkbox checked={d.appliesTo.includes(t.key)} onCheckedChange={() => setD({ ...d, appliesTo: toggle(d.appliesTo, t.key) })} />
                  {t.key}
                </Label>
              ))}
            </div>
            <p className="text-[12px] text-faint">Nothing ticked means any node type.</p>
          </div>
          <div className="flex flex-col gap-3">
            <span className="text-[12px] font-semibold text-muted-foreground">Permissions</span>
            {byService.map(([svc, perms]) => (
              <div key={svc} className="rounded-xl border">
                <div className="border-b px-3 py-2 text-[10px] font-bold tracking-wide text-faint uppercase">{svc}</div>
                <div className="flex flex-col">
                  {perms.map((p) => (
                    <Label key={p.key} className="flex cursor-pointer items-start gap-3 px-3 py-2 font-normal hover:bg-accent/50">
                      <Checkbox checked={d.permissions.includes(p.key)} onCheckedChange={() => setD({ ...d, permissions: toggle(d.permissions, p.key) })} className="mt-0.5" />
                      <span className="min-w-0">
                        <span className="block font-mono text-[12px]">{p.key}</span>
                        <span className="block text-[12px] text-faint">
                          {p.description || 'No description'} · scope {p.scope}
                        </span>
                      </span>
                    </Label>
                  ))}
                </div>
              </div>
            ))}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !d.name || d.permissions.length === 0}>
              {busy ? 'Saving…' : d.id ? 'Save role' : 'Create role'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function RoleRow({ role, onEdit, onDelete }: { role: Role; onEdit?: () => void; onDelete?: () => void }) {
  return (
    <div className="flex items-start justify-between gap-4 px-4 py-3">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[13px] font-semibold">{role.name}</span>
          <span className="font-mono text-[11px] text-faint">{role.slug}</span>
          <Pill>
            {role.grantCount} {role.grantCount === 1 ? 'grant' : 'grants'}
          </Pill>
        </div>
        {role.description && <p className="mt-0.5 text-[12px] text-muted-foreground">{role.description}</p>}
        <div className="mt-2 flex flex-wrap gap-1">
          {role.permissions.map((p) => (
            <span key={p} className="rounded-md bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
              {p}
            </span>
          ))}
        </div>
        <p className="mt-1.5 text-[11px] text-faint">Granted on {role.appliesTo.length ? role.appliesTo.join(', ') : 'any node'}</p>
      </div>
      <div className="flex shrink-0 gap-1">
        {onEdit && (
          <Button variant="ghost" size="icon-sm" onClick={onEdit} aria-label={`Edit ${role.name}`}>
            <Pencil />
          </Button>
        )}
        {onDelete && (
          <Button variant="ghost" size="icon-sm" onClick={onDelete} aria-label={`Delete ${role.name}`}>
            <Trash2 />
          </Button>
        )}
      </div>
    </div>
  )
}

export default function RolesPage() {
  const catalog = useClient(CatalogService)
  const { data, error, loading, reload } = useQuery(() => catalog.listRoles({}).then((r) => r.roles), [catalog])
  const [editing, setEditing] = useState<Draft | null>(null)
  const [deleting, setDeleting] = useState<Role | null>(null)

  const roles = data ?? []
  const builtin = roles.filter((r) => r.service === 'hayat')
  const custom = roles.filter((r) => r.service === '')
  const services = roles.filter((r) => r.service !== '' && r.service !== 'hayat')
  const draftOf = (r: Role): Draft => ({ id: r.id, name: r.name, description: r.description, appliesTo: r.appliesTo, permissions: r.permissions })

  return (
    <>
      <PageHeader
        title="Roles"
        desc="Named sets of permissions. Grant one to a person or a group on a node of the resource tree, and it reaches everything below unless you say otherwise."
        action={
          <Button size="lg" onClick={() => setEditing({ name: '', description: '', appliesTo: [], permissions: [] })}>
            <Plus /> Create role
          </Button>
        }
      />
      <div className="mt-7">
        {error ? (
          <ErrorState message={error} onRetry={reload} />
        ) : loading && !data ? (
          <LoadingState />
        ) : (
          <>
            <Section title="Built-in" desc="Hayat's own roles. You can change what they grant, not delete them.">
              <div className="divide-y rounded-xl border">
                {builtin.map((r) => (
                  <RoleRow key={r.id} role={r} onEdit={() => setEditing(draftOf(r))} />
                ))}
              </div>
            </Section>
            <Section title="Yours" desc="Roles made on this instance.">
              {custom.length === 0 ? (
                <EmptyState icon={ShieldHalf} title="No custom roles yet">
                  Combine permissions from several services into one role, such as "Finance lead".
                </EmptyState>
              ) : (
                <div className="divide-y rounded-xl border">
                  {custom.map((r) => (
                    <RoleRow key={r.id} role={r} onEdit={() => setEditing(draftOf(r))} onDelete={() => setDeleting(r)} />
                  ))}
                </div>
              )}
            </Section>
            <Section title="From services" desc="Default roles each service's contract suggests. They change with the contract.">
              {services.length === 0 ? (
                <p className="text-[13px] text-faint">No active contract suggests a role yet.</p>
              ) : (
                <div className="divide-y rounded-xl border">
                  {services.map((r) => (
                    <RoleRow key={r.id} role={r} />
                  ))}
                </div>
              )}
            </Section>
          </>
        )}
      </div>
      {editing && (
        <RoleDialog
          draft={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            void reload()
          }}
        />
      )}
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete ${deleting?.name}?`}
        body="Only a role that isn't granted anywhere can be deleted."
        confirm="Delete role"
        destructive
        onConfirm={async () => {
          if (!deleting) return
          await catalog.deleteRole({ id: deleting.id })
          toast.success('Role deleted')
          await reload()
        }}
      />
    </>
  )
}
