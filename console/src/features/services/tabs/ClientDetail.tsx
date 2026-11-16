import { useState } from 'react'
import { Link } from 'react-router'
import { toast } from 'sonner'
import { ArrowLeft, KeyRound, Power } from 'lucide-react'
import { RegistryService, type ServiceInfo } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDateTime } from '@/shared/lib/datetime'
import { useAuth } from '@/features/auth'
import { CopyField, DetailList, ErrorState, LoadingState, Pill, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Section } from '@/shared/ui/page/PageHeader'
import { ClientFields, type ClientValues } from '../components/ClientFields'
import { SecretDialog } from '../components/SecretDialog'
import { clientKind } from '../lib'

/** /services/:id/clients/:clientId — one sign-in client: its settings and what an app needs to use it. */
export function ClientDetail({ service: s, clientId }: { service: ServiceInfo; clientId: string }) {
  const registry = useClient(RegistryService)
  const { config } = useAuth()
  const { data, error, loading, reload } = useQuery(() => registry.getServiceClient({ serviceId: s.id, clientId }), [registry, s.id, clientId])
  const [edit, setEdit] = useState<ClientValues | null>(null)
  const [saving, setSaving] = useState(false)
  const [confirm, setConfirm] = useState<'rotate' | 'disable' | null>(null)
  const [secret, setSecret] = useState('')

  if (error) return <ErrorState message={error} onRetry={reload} />
  if (loading && !data) return <LoadingState />
  if (!data?.client) return null
  const c = data.client
  const editable = data.canManage && !c.builtin
  const values: ClientValues = edit ?? { name: c.name, redirectUris: c.redirectUris, postLogoutRedirectUris: c.postLogoutRedirectUris, grantTypes: c.grantTypes }
  const kind = clientKind(c)

  const save = async () => {
    setSaving(true)
    try {
      await registry.updateServiceClient({ serviceId: s.id, clientId: c.id, ...values })
      toast.success('Client saved')
      setEdit(null)
      await reload()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  const setDisabled = async (disabled: boolean) => {
    await registry.setServiceClientDisabled({ serviceId: s.id, clientId: c.id, disabled })
    toast.success(disabled ? 'Client turned off' : 'Client turned on')
    await reload()
  }

  return (
    <>
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex min-w-0 flex-col gap-2">
          <Link to={`/services/${s.id}/clients`} className="inline-flex items-center gap-1.5 text-[12px] text-muted-foreground hover:text-foreground">
            <ArrowLeft className="size-3.5" /> All clients
          </Link>
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-[18px] font-semibold tracking-[-0.3px]">{c.name}</h2>
            <Pill>{kind.label}</Pill>
            {c.builtin && <Pill>Built-in</Pill>}
            <StatusBadge status={c.disabled ? 'disabled' : 'active'} />
          </div>
        </div>
        {editable && (
          <div className="flex gap-2">
            {c.kind === 'confidential' && (
              <Button variant="outline" size="lg" onClick={() => setConfirm('rotate')}>
                <KeyRound /> Rotate secret
              </Button>
            )}
            {c.disabled ? (
              <Button
                variant="outline"
                size="lg"
                onClick={() => setDisabled(false).catch((err) => toast.error(errorMessage(err)))}
              >
                <Power /> Turn on
              </Button>
            ) : (
              <Button variant="outline" size="lg" className="text-destructive" onClick={() => setConfirm('disable')}>
                <Power /> Turn off
              </Button>
            )}
          </div>
        )}
      </div>

      <div className="mt-7 grid grid-cols-[minmax(0,1fr)_minmax(300px,380px)] gap-x-10 max-[1000px]:grid-cols-1">
        <Section title="Settings" desc={c.builtin ? 'The console client is managed by Hayat from HAYAT_CONSOLE_URL.' : 'Changes apply to the next sign-in.'}>
          <div className="rounded-xl border p-5">
            <div className="mb-5 flex flex-wrap items-center gap-2 rounded-[10px] bg-muted/60 px-3 py-2.5 text-[12px] text-muted-foreground">
              <span className="font-semibold text-foreground">{kind.label}</span>
              {kind.detail}
              <span className="ml-auto text-[11px] text-faint">Fixed once added.</span>
            </div>
            <ClientFields value={values} onChange={setEdit} disabled={!editable} />
            {editable && (
              <div className="mt-5 flex justify-end gap-2 border-t pt-4">
                <Button variant="outline" disabled={!edit || saving} onClick={() => setEdit(null)}>
                  Reset
                </Button>
                <Button disabled={!edit || saving || !values.name.trim()} onClick={save}>
                  {saving ? 'Saving…' : 'Save changes'}
                </Button>
              </div>
            )}
          </div>
        </Section>

        <div className="min-w-0 max-[1000px]:mt-9">
          <Section title="Connect the app" desc="What its OIDC library needs.">
            <div className="rounded-xl border p-4">
              <DetailList
                stacked
                items={[
                  ['Client id', <CopyField key="id" value={c.id} />],
                  ['Issuer', <CopyField key="iss" value={config.issuer} />],
                  ['Discovery', <CopyField key="d" value={`${config.issuer}/.well-known/openid-configuration`} />],
                  ['Auth method', c.kind === 'public' ? 'None, with PKCE (S256)' : 'client_secret_basic'],
                  ['Token audience', <span key="aud" className="font-mono text-[12px]">{`${c.id}, ${s.id}`}</span>],
                  ['Registered', formatDateTime(c.createdAt)],
                ]}
              />
              <p className="mt-4 text-[12px] text-faint">
                {s.displayName}'s server can accept a token from any of its sign-in clients by checking <span className="font-mono">{s.id}</span> in its audience.
              </p>
            </div>
          </Section>
        </div>
      </div>

      <ConfirmDialog
        open={confirm === 'rotate'}
        onOpenChange={(o) => !o && setConfirm(null)}
        title="Rotate the client secret?"
        body="A new secret is shown once. The current secret stops working immediately, so update the application right away."
        confirm="Rotate secret"
        destructive
        onConfirm={async () => {
          const res = await registry.rotateServiceClientSecret({ serviceId: s.id, clientId: c.id })
          setSecret(res.secret)
        }}
      />
      <ConfirmDialog
        open={confirm === 'disable'}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={`Turn off ${c.name}?`}
        body="Nobody can sign in through it, and its refresh tokens stop working, until it's turned on again."
        confirm="Turn off"
        destructive
        onConfirm={() => setDisabled(true)}
      />
      {secret && <SecretDialog clientId={c.id} secret={secret} onClose={() => setSecret('')} />}
    </>
  )
}
