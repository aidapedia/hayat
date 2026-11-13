import { useEffect, useMemo, useState } from 'react'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { toast } from 'sonner'
import { Check } from 'lucide-react'
import { TenantService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { TreeAdminService, type Node, type Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { cn } from '@/shared/lib/utils'
import { Field, LoadingState, SearchInput } from '@/shared/ui/bits'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Switch } from '@/shared/ui/switch'
import { allMembers, allTenants } from './api'
import { Shell, useSubmit } from './dialogs'
import { NodeIcon } from './NodeIcon'
import { GROUP, isContainer, nodePath, TENANT, typeLabel } from './tree'

/** "Tenant, Group" — the node types this role may be granted on. */
const typesLine = (appliesTo: string[]) => (appliesTo.length === 0 ? 'any node' : appliesTo.map(typeLabel).join(', '))

/**
 * Grant one role, starting from the role instead of from a node: pick the
 * tenant, the node inside it the role may be granted on, and who gets it.
 * The node side of the same thing is the Grant button on a node (GrantDialog),
 * and both end in TreeAdminService.AddGrant.
 */
export function GrantRoleDialog({ role, onClose, onDone }: { role: Role; onClose: () => void; onDone: () => void }) {
  const tenantsApi = useClient(TenantService)
  const tree = useClient(TreeAdminService)

  const tenants = useQuery(() => allTenants(tenantsApi), [tenantsApi])
  const [tenantId, setTenantId] = useState('')
  // One tenant to choose from is not a choice: stand in it.
  useEffect(() => {
    const list = tenants.data
    if (list && !tenantId && list.length === 1) setTenantId(list[0].id)
  }, [tenants.data, tenantId])

  const nodes = useQuery(
    () => (tenantId ? tree.getTenantTree({ tenantId }).then((r) => r.nodes) : Promise.resolve([] as Node[])),
    [tree, tenantId],
  )
  const members = useQuery(() => (tenantId ? allMembers(tenantsApi, tenantId) : Promise.resolve([])), [tenantsApi, tenantId])

  const [nodeId, setNodeId] = useState('')
  const [query, setQuery] = useState('')
  const [kind, setKind] = useState<'user' | 'group'>('user')
  const [principal, setPrincipal] = useState('')
  const [inherit, setInherit] = useState(true)
  const [expires, setExpires] = useState('')

  const list = nodes.data ?? []
  const byId = useMemo(() => new Map(list.map((n) => [n.id, n])), [list])
  // Only where the role means something: its own types, or anywhere when it has none.
  const grantable = useMemo(
    () =>
      list
        .filter((n) => role.appliesTo.length === 0 || role.appliesTo.includes(n.type))
        .sort((a, b) => a.depth - b.depth || a.name.localeCompare(b.name)),
    [list, role.appliesTo],
  )
  const candidates = useMemo(() => {
    const q = query.trim().toLowerCase()
    return q ? grantable.filter((n) => nodePath(byId, n).join(' / ').toLowerCase().includes(q)) : grantable
  }, [grantable, byId, query])

  const node = nodeId ? byId.get(nodeId) : undefined
  const groups = list.filter((n) => n.type === GROUP || n.type === TENANT)
  const tenantName = list.find((n) => n.type === TENANT)?.name ?? tenants.data?.find((t) => t.id === tenantId)?.name ?? ''

  const { busy, submit } = useSubmit(async () => {
    await tree.addGrant({
      nodeId,
      principalType: kind,
      principalId: principal,
      roleId: role.id,
      inherit,
      expiresAt: expires ? timestampFromDate(new Date(expires)) : undefined,
    })
    toast.success(`${role.name} granted on ${node?.name ?? 'the node'}`)
  }, onDone)

  const loadingTree = !!tenantId && nodes.loading
  return (
    <Shell
      title={`Grant ${role.name}`}
      desc={`A grant is this role, to one person or group, at one node. Grantable on ${typesLine(role.appliesTo)}.`}
      submitLabel="Grant role"
      className="sm:max-w-lg"
      busy={busy}
      disabled={!nodeId || !principal}
      onClose={onClose}
      onSubmit={submit}
    >
      <Field label="Tenant" hint={tenants.error ?? undefined}>
        <Select
          value={tenantId}
          onValueChange={(v) => {
            setTenantId(v)
            setNodeId('')
            setPrincipal('')
            setQuery('')
          }}
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder={tenants.loading ? 'Loading tenants…' : 'Choose a tenant'} />
          </SelectTrigger>
          <SelectContent>
            {(tenants.data ?? []).map((t) => (
              <SelectItem key={t.id} value={t.id}>
                {t.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>

      <Field label="On which node" hint={node ? nodePath(byId, node).join(' › ') : 'The grant applies here, and below it when it inherits.'}>
        {!tenantId ? (
          <p className="rounded-[10px] border border-dashed px-3 py-4 text-center text-[12px] text-faint">Pick a tenant first.</p>
        ) : loadingTree ? (
          <LoadingState label="Reading the tree…" />
        ) : nodes.error ? (
          <p className="text-[12px] text-destructive">{nodes.error}</p>
        ) : (
          <>
            {grantable.length > 8 && <SearchInput placeholder="Search nodes" value={query} onChange={(e) => setQuery(e.target.value)} />}
            <ul className="max-h-56 overflow-y-auto rounded-[10px] border">
              {candidates.length === 0 ? (
                <li className="px-3 py-6 text-center text-[12px] text-faint">
                  {query ? 'No node matches.' : `Nothing in ${tenantName || 'this tenant'} takes this role: it can only be granted on ${typesLine(role.appliesTo)}.`}
                </li>
              ) : (
                candidates.map((n) => {
                  const on = n.id === nodeId
                  const trail = nodePath(byId, n).slice(0, -1).join(' › ')
                  return (
                    <li key={n.id}>
                      <button
                        type="button"
                        onClick={() => setNodeId(n.id)}
                        className={cn('flex w-full items-center gap-2.5 px-3 py-2 text-left hover:bg-accent/50', on && 'bg-accent/60')}
                      >
                        <NodeIcon type={n.type} className="text-muted-foreground" />
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-[13px] font-medium">{n.name}</span>
                          <span className="block truncate text-[11px] text-faint">
                            {typeLabel(n.type)}
                            {trail && ` · ${trail}`}
                          </span>
                        </span>
                        {on && <Check className="size-3.5 shrink-0 text-primary" strokeWidth={2.6} />}
                      </button>
                    </li>
                  )
                })
              )}
            </ul>
          </>
        )}
      </Field>

      <Field label="To">
        <div className="flex gap-2">
          <Select
            value={kind}
            onValueChange={(v) => {
              setKind(v as 'user' | 'group')
              setPrincipal('')
            }}
          >
            <SelectTrigger className="w-28">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="user">Person</SelectItem>
              <SelectItem value="group">Group</SelectItem>
            </SelectContent>
          </Select>
          <Select value={principal} onValueChange={setPrincipal} disabled={!tenantId}>
            <SelectTrigger className="min-w-0 flex-1">
              <SelectValue placeholder={kind === 'user' ? 'Choose a member' : 'Choose a group'} />
            </SelectTrigger>
            <SelectContent>
              {kind === 'user'
                ? (members.data ?? []).map((m) => (
                    <SelectItem key={m.user!.id} value={m.user!.id}>
                      {m.user!.name ? `${m.user!.name} · ${m.user!.email}` : m.user!.email}
                    </SelectItem>
                  ))
                : groups.map((g) => (
                    <SelectItem key={g.id} value={g.id}>
                      {g.type === TENANT ? `Everyone in ${g.name}` : nodePath(byId, g).slice(1).join(' › ') || g.name}
                    </SelectItem>
                  ))}
            </SelectContent>
          </Select>
        </div>
      </Field>

      <Label className="flex items-start justify-between gap-4 rounded-[10px] border px-3 py-2.5 font-normal">
        <span>
          <span className="block text-[13px] font-medium">Reaches everything below</span>
          <span className="block text-[12px] text-faint">
            {node && !isContainer(node.type) ? 'Nothing is filed under this node; it applies here either way.' : 'Off: the role applies on the chosen node only.'}
          </span>
        </span>
        <Switch checked={inherit} onCheckedChange={setInherit} />
      </Label>

      <Field label="Expires" htmlFor="grant-role-expiry" hint="Optional. For temporary access.">
        <Input id="grant-role-expiry" type="datetime-local" value={expires} onChange={(e) => setExpires(e.target.value)} />
      </Field>
    </Shell>
  )
}
