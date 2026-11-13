import { useMemo, useState, type FormEvent } from 'react'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { toast } from 'sonner'
import { CatalogService, TreeAdminService, type Node } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import type { Member } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { cn } from '@/shared/lib/utils'
import { Field } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Switch } from '@/shared/ui/switch'
import { nodePath } from './tree'


/** A small dialog with one form, the shape every tree dialog shares. */
export function Shell({
  title,
  desc,
  submitLabel,
  busy,
  disabled,
  className,
  onClose,
  onSubmit,
  children,
}: {
  title: string
  desc?: string
  submitLabel: string
  busy: boolean
  disabled?: boolean
  /** Widens the dialog where one form needs more room than a few fields. */
  className?: string
  onClose: () => void
  onSubmit: (e: FormEvent) => void
  children: React.ReactNode
}) {
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className={cn('sm:max-w-md', className)}>
        <form onSubmit={onSubmit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">{title}</DialogTitle>
            {desc && <DialogDescription className="text-[13px]">{desc}</DialogDescription>}
          </DialogHeader>
          {children}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || disabled}>
              {busy ? 'Saving…' : submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/** Runs one form submit: busy while it goes, a toast if it fails, `done` when it worked. */
export function useSubmit(run: () => Promise<void>, done: () => void) {
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await run()
      done()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return { busy, submit }
}

export function CreateGroupDialog({ parent, onClose, onDone }: { parent: Node; onClose: () => void; onDone: (n: Node) => void }) {
  const tree = useClient(TreeAdminService)
  const [name, setName] = useState('')
  const { busy, submit } = useSubmit(
    async () => {
      const res = await tree.createGroup({ parentId: parent.id, name })
      toast.success(`${name} created`)
      onDone(res.node!)
    },
    () => {},
  )
  return (
    <Shell title={`New group in ${parent.name}`} desc="A group is a folder for resources and a list of people at once." submitLabel="Create group" busy={busy} disabled={!name.trim()} onClose={onClose} onSubmit={submit}>
      <Field label="Name" htmlFor="group-name">
        <Input id="group-name" value={name} onChange={(e) => setName(e.target.value)} autoFocus required />
      </Field>
    </Shell>
  )
}

export function RenameGroupDialog({ node, onClose, onDone }: { node: Node; onClose: () => void; onDone: () => void }) {
  const tree = useClient(TreeAdminService)
  const [name, setName] = useState(node.name)
  const { busy, submit } = useSubmit(async () => {
    await tree.renameGroup({ id: node.id, name })
    toast.success('Group renamed')
  }, onDone)
  return (
    <Shell title="Rename group" submitLabel="Rename" busy={busy} disabled={!name.trim() || name === node.name} onClose={onClose} onSubmit={submit}>
      <Field label="Name" htmlFor="rename">
        <Input id="rename" value={name} onChange={(e) => setName(e.target.value)} autoFocus required />
      </Field>
    </Shell>
  )
}

export function MoveDialog({ node, nodes, onClose, onDone }: { node: Node; nodes: Node[]; onClose: () => void; onDone: () => void }) {
  const tree = useClient(TreeAdminService)
  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes])
  // Anything under the node can't be its new parent.
  const inside = (n: Node) => {
    for (let cur: Node | undefined = n; cur; cur = byId.get(cur.parentId)) if (cur.id === node.id) return true
    return false
  }
  const pathOf = (n: Node) => nodePath(byId, n).join(' / ')
  const targets = nodes.filter((n) => (n.type === 'hayat/tenant' || n.type === 'hayat/group') && !inside(n) && n.id !== node.parentId)
  const [dest, setDest] = useState('')
  const { busy, submit } = useSubmit(async () => {
    await tree.moveNode({ id: node.id, parentId: dest })
    toast.success(`${node.name} moved`)
  }, onDone)
  return (
    <Shell
      title={`Move ${node.name}`}
      desc="Everything filed under it moves too. Moving a group changes no roster: nobody gains access by its new place."
      submitLabel="Move here"
      busy={busy}
      disabled={!dest}
      onClose={onClose}
      onSubmit={submit}
    >
      <Field label="New place">
        <Select value={dest} onValueChange={setDest}>
          <SelectTrigger className="w-full">
            <SelectValue placeholder="Choose a group" />
          </SelectTrigger>
          <SelectContent>
            {targets.map((t) => (
              <SelectItem key={t.id} value={t.id}>
                {pathOf(t)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
    </Shell>
  )
}

export function GrantDialog({
  node,
  nodes,
  members,
  tenantName,
  onClose,
  onDone,
}: {
  node: Node
  nodes: Node[]
  members: Member[]
  tenantName: string
  onClose: () => void
  onDone: () => void
}) {
  const tree = useClient(TreeAdminService)
  const catalog = useClient(CatalogService)
  const { data: roles } = useQuery(() => catalog.listRoles({}).then((r) => r.roles), [catalog])
  const [kind, setKind] = useState<'user' | 'group'>('user')
  const [principal, setPrincipal] = useState('')
  const [role, setRole] = useState('')
  const [inherit, setInherit] = useState(true)
  const [expires, setExpires] = useState('')
  const usable = (roles ?? []).filter((r) => r.appliesTo.length === 0 || r.appliesTo.includes(node.type))
  const groups = nodes.filter((n) => n.type === 'hayat/group' || n.type === 'hayat/tenant')
  const { busy, submit } = useSubmit(async () => {
    await tree.addGrant({
      nodeId: node.id,
      principalType: kind,
      principalId: principal,
      roleId: role,
      inherit,
      expiresAt: expires ? timestampFromDate(new Date(expires)) : undefined,
    })
    toast.success('Access granted')
  }, onDone)
  return (
    <Shell title={`Grant a role on ${node.name}`} submitLabel="Grant" busy={busy} disabled={!principal || !role} onClose={onClose} onSubmit={submit}>
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
          <Select value={principal} onValueChange={setPrincipal}>
            <SelectTrigger className="min-w-0 flex-1">
              <SelectValue placeholder={kind === 'user' ? 'Choose a member' : 'Choose a group'} />
            </SelectTrigger>
            <SelectContent>
              {kind === 'user'
                ? members.map((m) => (
                    <SelectItem key={m.user!.id} value={m.user!.id}>
                      {m.user!.name ? `${m.user!.name} · ${m.user!.email}` : m.user!.email}
                    </SelectItem>
                  ))
                : groups.map((g) => (
                    <SelectItem key={g.id} value={g.id}>
                      {g.type === 'hayat/tenant' ? `Everyone in ${tenantName}` : g.name}
                    </SelectItem>
                  ))}
            </SelectContent>
          </Select>
        </div>
      </Field>
      <Field label="Role" hint="Only roles that can be granted on this kind of node are listed.">
        <Select value={role} onValueChange={setRole}>
          <SelectTrigger className="w-full">
            <SelectValue placeholder="Choose a role" />
          </SelectTrigger>
          <SelectContent>
            {usable.map((r) => (
              <SelectItem key={r.id} value={r.id}>
                {r.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <Label className="flex items-start justify-between gap-4 rounded-[10px] border px-3 py-2.5 font-normal">
        <span>
          <span className="block text-[13px] font-medium">Reaches everything below</span>
          <span className="block text-[12px] text-faint">Off: the role applies on this node only.</span>
        </span>
        <Switch checked={inherit} onCheckedChange={setInherit} />
      </Label>
      <Field label="Expires" htmlFor="grant-expiry" hint="Optional. For temporary access.">
        <Input id="grant-expiry" type="datetime-local" value={expires} onChange={(e) => setExpires(e.target.value)} />
      </Field>
    </Shell>
  )
}

export function AddGroupMemberDialog({ group, members, already, onClose, onDone }: { group: Node; members: Member[]; already: Set<string>; onClose: () => void; onDone: () => void }) {
  const tree = useClient(TreeAdminService)
  const [user, setUser] = useState('')
  const candidates = members.filter((m) => !already.has(m.user!.id))
  const { busy, submit } = useSubmit(async () => {
    await tree.addGroupMember({ groupId: group.id, userId: user })
    toast.success('Added to the group')
  }, onDone)
  return (
    <Shell
      title={`Add someone to ${group.name}`}
      desc="Grants made to this group reach them, and so does anything filed in it that gives access by placement."
      submitLabel="Add"
      busy={busy}
      disabled={!user}
      onClose={onClose}
      onSubmit={submit}
    >
      <Field label="Tenant member">
        <Select value={user} onValueChange={setUser}>
          <SelectTrigger className="w-full">
            <SelectValue placeholder={candidates.length ? 'Choose a member' : 'Everyone is already in this group'} />
          </SelectTrigger>
          <SelectContent>
            {candidates.map((m) => (
              <SelectItem key={m.user!.id} value={m.user!.id}>
                {m.user!.name ? `${m.user!.name} · ${m.user!.email}` : m.user!.email}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
    </Shell>
  )
}
