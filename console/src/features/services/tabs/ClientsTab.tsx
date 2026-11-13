import { useState } from 'react'
import { useNavigate } from 'react-router'
import { AppWindow, KeyRound, Plus, Server } from 'lucide-react'
import { RegistryService, type ServiceInfo } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { CopyField, EmptyState, ErrorState, LoadingState, Pill, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Section } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Pagination } from '@/shared/ui/table/Pagination'
import { usePagination } from '@/shared/ui/table/usePagination'
import { AddClientSheet } from '../components/AddClientSheet'
import { CredentialsDialog } from '../CredentialsDialog'
import { clientKind, returnsTo } from '../lib'

/** A service's API credential, and the sign-in clients people use to reach it. */
export function ClientsTab({ service: s, canManage, onChanged }: { service: ServiceInfo; canManage: boolean; onChanged: () => unknown }) {
  const registry = useClient(RegistryService)
  const navigate = useNavigate()
  const { data, error, loading, reload } = useQuery(() => registry.listServiceClients({ serviceId: s.id }), [registry, s.id])
  const { pageRows, pager } = usePagination(data?.clients ?? [])
  const [adding, setAdding] = useState(false)
  const [rotating, setRotating] = useState(false)
  const [secret, setSecret] = useState('')

  return (
    <>
      {!s.builtin && (
        <Section title="API credential" desc={`How ${s.displayName}'s server calls Hayat as ${s.id}: decisions, lookups, Watch, publishing contracts. Every service has exactly one.`}>
          <div className="flex flex-wrap items-end gap-5 rounded-xl border p-4">
            <div className="flex min-w-[220px] flex-1 items-center gap-3">
              <span className="inline-flex size-10 items-center justify-center rounded-[10px] bg-muted text-muted-foreground">
                <Server className="size-[18px]" />
              </span>
              <div>
                <div className="flex items-center gap-2 text-[13px] font-semibold">
                  Service API <Pill>Machine</Pill>
                </div>
                <div className="text-[12px] text-muted-foreground">Client credentials</div>
              </div>
            </div>
            <div className="min-w-[260px] flex-[2]">
              <div className="mb-1.5 text-[12px] text-muted-foreground">Client id</div>
              <CopyField value={s.clientId} />
            </div>
            {canManage && (
              <Button variant="outline" onClick={() => setRotating(true)}>
                <KeyRound /> Rotate secret
              </Button>
            )}
          </div>
        </Section>
      )}

      <Section
        title="Sign-in clients"
        desc={`The apps people sign in to ${s.displayName} through. Their tokens name ${s.id} in their audience.`}
        action={
          canManage && (
            <Button size="lg" onClick={() => setAdding(true)}>
              <Plus /> Add sign-in client
            </Button>
          )
        }
      >
        {error ? (
          <ErrorState message={error} onRetry={reload} />
        ) : loading && !data ? (
          <LoadingState />
        ) : !data || data.clients.length === 0 ? (
          <EmptyState icon={AppWindow} title="No sign-in clients yet">
            Add one for each app people sign in through: a web server, a browser app, or a native or command-line app.
          </EmptyState>
        ) : (
          <div className="overflow-hidden rounded-xl border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="text-[11px]">Client</TableHead>
                  <TableHead className="text-[11px]">Kind</TableHead>
                  <TableHead className="text-[11px]">Returns to</TableHead>
                  <TableHead className="text-[11px]">Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pageRows.map((c) => (
                  <TableRow key={c.id} className="cursor-pointer" onClick={() => navigate(`/services/${s.id}/clients/${c.id}`)}>
                    <TableCell>
                      <div className="flex items-center gap-2 text-[13px] font-semibold">
                        {c.name}
                        {c.builtin && <Pill>Built-in</Pill>}
                      </div>
                      <div className="font-mono text-[12px] text-faint">{c.id}</div>
                    </TableCell>
                    <TableCell>
                      <div className="text-[12px]">{clientKind(c).label}</div>
                      <div className="text-[11px] text-faint">{clientKind(c).detail}</div>
                    </TableCell>
                    <TableCell className="max-w-[280px] truncate font-mono text-[12px] text-muted-foreground">{returnsTo(c)}</TableCell>
                    <TableCell>
                      <StatusBadge status={c.disabled ? 'disabled' : 'active'} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <Pagination {...pager} className="border-t" />
          </div>
        )}
      </Section>

      <AddClientSheet
        service={s}
        open={adding}
        onOpenChange={setAdding}
        onAdded={async () => {
          await reload()
          await onChanged()
        }}
      />
      <ConfirmDialog
        open={rotating}
        onOpenChange={setRotating}
        title={`Rotate ${s.displayName}'s API secret?`}
        body="The current secret stops working at once, and the service can't ask Hayat for decisions until it has the new one. The new secret is shown once."
        confirm="Rotate secret"
        destructive
        onConfirm={async () => {
          const res = await registry.rotateServiceSecret({ serviceId: s.id })
          setSecret(res.clientSecret)
        }}
      />
      {secret && <CredentialsDialog clientId={s.clientId} secret={secret} onClose={() => setSecret('')} />}
    </>
  )
}
