import { useMemo, useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { Bot, MoreHorizontal, Pause, Play, Trash2, UserCog } from 'lucide-react'
import { AgentService, type Agent } from '@/shared/api/gen/hayat/admin/v1/agents_pb'
import { TenantService, type Tenant } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDate } from '@/shared/lib/datetime'
import { EmptyState, ErrorState, Field, LoadingState, LogoTile, Pill, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Combobox, type ComboboxOption } from '@/shared/ui/combobox'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/shared/ui/dropdown-menu'
import { Input } from '@/shared/ui/input'
import { Section } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

/** The people on the tenant's roster, as sponsor options. Only an active person can sponsor. */
function useSponsorOptions(tenantId: string, open: boolean) {
  const tenants = useClient(TenantService)
  const { data } = useQuery(() => (open ? tenants.listMembers({ tenantId, pageSize: 500 }) : Promise.resolve(null)), [tenants, tenantId, open])
  return useMemo<ComboboxOption[]>(
    () =>
      (data?.members ?? [])
        .filter((m) => m.user?.status === 'active')
        .map((m) => ({ value: m.user!.id, label: m.user!.name || m.user!.email, hint: m.user!.email })),
    [data],
  )
}

const splitServices = (raw: string) =>
  raw
    .split(/[\s,]+/)
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean)

function AddAgentDialog({ tenantId, open, onOpenChange, onAdded }: { tenantId: string; open: boolean; onOpenChange: (o: boolean) => void; onAdded: () => void }) {
  const agents = useClient(AgentService)
  const sponsors = useSponsorOptions(tenantId, open)
  const [name, setName] = useState('')
  const [handle, setHandle] = useState('')
  const [sponsor, setSponsor] = useState('')
  const [services, setServices] = useState('tether')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const res = await agents.createAgent({ tenantId, name, handle, sponsorUserId: sponsor, services: splitServices(services) })
      toast.success(`@${res.agent?.handle} added`)
      setName('')
      setHandle('')
      setSponsor('')
      onOpenChange(false)
      onAdded()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">Add an agent</DialogTitle>
            <DialogDescription className="text-[13px]">
              An agent never signs in and has no email. It answers to a person here; if they leave, it pauses. Its role, model and tools are set in
              the services it works in.
            </DialogDescription>
          </DialogHeader>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Name" htmlFor="a-name">
              <Input id="a-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Juno" required autoFocus />
            </Field>
            <Field label="Handle" htmlFor="a-handle" hint="Unique in this tenant.">
              <Input id="a-handle" value={handle} onChange={(e) => setHandle(e.target.value)} placeholder="juno" required />
            </Field>
          </div>
          <Field label="Sponsor" htmlFor="a-sponsor" hint="The person who answers for it.">
            <Combobox id="a-sponsor" value={sponsor} onChange={setSponsor} options={sponsors} placeholder="Pick a member" />
          </Field>
          <Field label="Works in" htmlFor="a-services" hint="The services that may act as it, by namespace. Separate with commas.">
            <Input id="a-services" value={services} onChange={(e) => setServices(e.target.value)} className="font-mono" />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !name || !handle || !sponsor}>
              {busy ? 'Adding…' : 'Add agent'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ReassignDialog({ agent, onOpenChange, onDone }: { agent: Agent | null; onOpenChange: (o: boolean) => void; onDone: () => void }) {
  const agents = useClient(AgentService)
  const sponsors = useSponsorOptions(agent?.tenantId ?? '', !!agent)
  const [sponsor, setSponsor] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!agent) return
    setBusy(true)
    try {
      await agents.reassignAgent({ id: agent.id, sponsorUserId: sponsor })
      toast.success('Sponsor changed')
      onOpenChange(false)
      onDone()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={!!agent} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">Change {agent?.name}'s sponsor</DialogTitle>
            <DialogDescription className="text-[13px]">
              The audit log keeps the sponsor's name on everything the agent does. An agent paused because its sponsor left resumes.
            </DialogDescription>
          </DialogHeader>
          <Field label="New sponsor" htmlFor="r-sponsor">
            <Combobox id="r-sponsor" value={sponsor} onChange={setSponsor} options={sponsors} placeholder="Pick a member" />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !sponsor}>
              {busy ? 'Saving…' : 'Change sponsor'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

const REASON: Record<string, string> = { sponsor_left: 'Sponsor left' }

type Pending = { title: string; body: string; confirm: string; destructive?: boolean; run: () => Promise<void> }

/**
 * A tenant's agents: members that aren't people. Anyone on the roster sees
 * them; hayat.agents.manage adds and changes them (the server checks each).
 * Their grants are made in the Resource Tree, like anyone's.
 */
export function TenantAgents({ tenant: t }: { tenant: Tenant }) {
  const agents = useClient(AgentService)
  const { data, error, loading, reload } = useQuery(() => agents.listAgents({ tenantId: t.id }), [agents, t.id])
  const [adding, setAdding] = useState(false)
  const [reassigning, setReassigning] = useState<Agent | null>(null)
  const [pending, setPending] = useState<Pending | null>(null)
  const canManage = !!data?.canManage

  const setStatus = async (a: Agent, status: string, done: string) => {
    try {
      await agents.setAgentStatus({ id: a.id, status })
      toast.success(done)
      reload()
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }

  return (
    <Section
      title="Agents"
      desc="Members that aren't people. Each answers to a sponsor here and works only in the services named. What they may do is granted in the Resource Tree."
      action={
        canManage && (
          <Button onClick={() => setAdding(true)}>
            <Bot /> Add agent
          </Button>
        )
      }
    >
      {error ? (
        <ErrorState message={error} onRetry={reload} />
      ) : loading && !data ? (
        <LoadingState />
      ) : data && data.agents.length === 0 ? (
        <EmptyState icon={Bot} title="No agents yet">
          {canManage ? 'Add one to let a service like Tether act as it.' : 'Someone who manages this tenant can add one.'}
        </EmptyState>
      ) : (
        <div className="overflow-hidden rounded-xl border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-[11px]">Agent</TableHead>
                <TableHead className="text-[11px]">Sponsor</TableHead>
                <TableHead className="text-[11px]">Works in</TableHead>
                <TableHead className="text-[11px]">Groups</TableHead>
                <TableHead className="text-[11px]">Status</TableHead>
                <TableHead className="text-[11px]">Added</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data?.agents.map((a) => (
                <TableRow key={a.id}>
                  <TableCell>
                    <div className="flex items-center gap-2.5">
                      <LogoTile name={a.name} size={28} className="rounded-lg" />
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5 text-[13px] font-semibold">
                          {a.name} <Pill>Agent</Pill>
                        </div>
                        <div className="font-mono text-[11px] text-faint">@{a.handle}</div>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="text-[12px]">{a.sponsor?.name || a.sponsor?.email || <span className="text-faint">—</span>}</TableCell>
                  <TableCell className="font-mono text-[12px] text-muted-foreground">{a.services.join(', ') || '—'}</TableCell>
                  <TableCell className="text-[12px] text-muted-foreground">{a.groups.map((g) => g.name).join(', ') || '—'}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1.5">
                      <StatusBadge status={a.status} />
                      {a.statusReason && <span className="text-[11px] text-faint">{REASON[a.statusReason] ?? a.statusReason}</span>}
                    </div>
                  </TableCell>
                  <TableCell className="text-[12px] text-muted-foreground">{formatDate(a.createdAt)}</TableCell>
                  <TableCell className="text-right">
                    {canManage && (
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${a.name}`}>
                            <MoreHorizontal />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-52">
                          {a.status === 'active' ? (
                            <DropdownMenuItem className="text-[13px]" onSelect={() => setStatus(a, 'paused', `${a.name} paused`)}>
                              <Pause /> Pause
                            </DropdownMenuItem>
                          ) : (
                            <DropdownMenuItem className="text-[13px]" onSelect={() => setStatus(a, 'active', `${a.name} resumed`)}>
                              <Play /> Resume
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuItem className="text-[13px]" onSelect={() => setReassigning(a)}>
                            <UserCog /> Change sponsor
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            className="text-[13px]"
                            onSelect={() =>
                              setPending({
                                title: `Remove ${a.name}?`,
                                body: 'Its groups and every grant made to it go too. Services stop being able to act as it.',
                                confirm: 'Remove agent',
                                destructive: true,
                                run: async () => {
                                  await agents.deleteAgent({ id: a.id })
                                  toast.success('Agent removed')
                                  reload()
                                },
                              })
                            }
                          >
                            <Trash2 /> Remove
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <AddAgentDialog tenantId={t.id} open={adding} onOpenChange={setAdding} onAdded={reload} />
      <ReassignDialog agent={reassigning} onOpenChange={(o) => !o && setReassigning(null)} onDone={reload} />
      <ConfirmDialog
        open={!!pending}
        onOpenChange={(o) => !o && setPending(null)}
        title={pending?.title ?? ''}
        body={pending?.body ?? ''}
        confirm={pending?.confirm ?? 'Confirm'}
        destructive={pending?.destructive}
        onConfirm={async () => {
          if (pending) await pending.run()
        }}
      />
    </Section>
  )
}
