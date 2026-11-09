import { useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { AppWindow, Plus } from 'lucide-react'
import { ClientService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { EmptyState, ErrorState, LoadingState, Pill, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { ClientForm } from './ClientForm'
import { SecretDialog } from './SecretDialog'

export const grantLabel: Record<string, string> = {
  authorization_code: 'Sign-in',
  refresh_token: 'Refresh',
  client_credentials: 'Client credentials',
}

export default function ClientsPage() {
  const clients = useClient(ClientService)
  const navigate = useNavigate()
  const { data, error, loading, reload } = useQuery(() => clients.listClients({}).then((r) => r.clients), [clients])
  const [creating, setCreating] = useState(false)
  const [created, setCreated] = useState<{ id: string; secret: string } | null>(null)

  return (
    <>
      <PageHeader
        title="OAuth clients"
        desc="Applications that sign people in with Hayat. Each gets a client id, and a server gets a secret."
        action={
          <Button size="lg" onClick={() => setCreating(true)}>
            <Plus /> Register client
          </Button>
        }
      />
      <div className="mt-7">
        {error ? (
          <ErrorState message={error} onRetry={reload} />
        ) : loading && !data ? (
          <LoadingState />
        ) : !data || data.length === 0 ? (
          <EmptyState icon={AppWindow} title="No clients yet" />
        ) : (
          <div className="overflow-x-auto rounded-xl border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="text-[11px]">Client</TableHead>
                  <TableHead className="text-[11px]">Kind</TableHead>
                  <TableHead className="text-[11px]">Grants</TableHead>
                  <TableHead className="text-[11px]">Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.map((c) => (
                  <TableRow key={c.id} className="cursor-pointer" onClick={() => navigate(`/clients/${c.id}`)}>
                    <TableCell>
                      <div className="flex items-center gap-2 text-[13px] font-semibold">
                        {c.name}
                        {c.builtin && <Pill>Built-in</Pill>}
                      </div>
                      <div className="font-mono text-[12px] text-faint">{c.id}</div>
                    </TableCell>
                    <TableCell className="text-[12px] text-muted-foreground capitalize">{c.kind}</TableCell>
                    <TableCell className="text-[12px] text-muted-foreground">{c.grantTypes.map((g) => grantLabel[g] ?? g).join(', ')}</TableCell>
                    <TableCell>
                      <StatusBadge status={c.disabled ? 'disabled' : 'active'} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      <Dialog open={creating} onOpenChange={setCreating}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle className="text-[15px]">Register a client</DialogTitle>
            <DialogDescription className="text-[13px]">The application then signs people in with Hayat's discovery document and this client id.</DialogDescription>
          </DialogHeader>
          <ClientForm
            initial={{ name: '', kind: 'confidential', redirectUris: [], postLogoutRedirectUris: [], grantTypes: ['authorization_code', 'refresh_token'] }}
            submitLabel="Register client"
            onCancel={() => setCreating(false)}
            onSubmit={async (v) => {
              try {
                const res = await clients.createClient(v)
                setCreating(false)
                await reload()
                if (res.secret) setCreated({ id: res.client!.id, secret: res.secret })
                else navigate(`/clients/${res.client!.id}`)
                toast.success(`${res.client!.name} registered`)
              } catch (err) {
                toast.error(errorMessage(err))
              }
            }}
          />
        </DialogContent>
      </Dialog>

      {created && (
        <SecretDialog
          clientId={created.id}
          secret={created.secret}
          onClose={() => {
            const id = created.id
            setCreated(null)
            navigate(`/clients/${id}`)
          }}
        />
      )}
    </>
  )
}
