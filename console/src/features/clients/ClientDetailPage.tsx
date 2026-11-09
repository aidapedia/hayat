import { useState } from 'react'
import { useParams } from 'react-router'
import { toast } from 'sonner'
import { KeyRound, Power } from 'lucide-react'
import { ClientService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDateTime } from '@/shared/lib/datetime'
import { useAuth } from '@/features/auth'
import { CopyField, DetailList, ErrorState, LoadingState, Pill, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { PageHeader, Section } from '@/shared/ui/page/PageHeader'
import { ClientForm } from './ClientForm'
import { SecretDialog } from './SecretDialog'

export default function ClientDetailPage() {
  const { id = '' } = useParams()
  const clients = useClient(ClientService)
  const { config } = useAuth()
  const { data, error, loading, reload } = useQuery(() => clients.getClient({ id }).then((r) => r.client), [clients, id])
  const [confirm, setConfirm] = useState<'rotate' | 'disable' | null>(null)
  const [secret, setSecret] = useState('')

  if (error) return <ErrorState message={error} onRetry={reload} />
  if (loading && !data) return <LoadingState />
  if (!data) return null
  const c = data

  return (
    <>
      <PageHeader
        title={c.name}
        crumbs={[{ label: c.name }]}
        meta={
          <>
            <Pill className="capitalize">{c.kind}</Pill>
            {c.builtin && <Pill>Built-in</Pill>}
            {c.disabled && <StatusBadge status="disabled" />}
          </>
        }
        action={
          !c.builtin && (
            <>
              {c.kind === 'confidential' && (
                <Button variant="outline" size="lg" onClick={() => setConfirm('rotate')}>
                  <KeyRound /> Rotate secret
                </Button>
              )}
              {c.disabled ? (
                <Button
                  variant="outline"
                  size="lg"
                  onClick={async () => {
                    try {
                      await clients.setClientDisabled({ id, disabled: false })
                      toast.success('Client turned on')
                      await reload()
                    } catch (err) {
                      toast.error(errorMessage(err))
                    }
                  }}
                >
                  <Power /> Turn on
                </Button>
              ) : (
                <Button variant="destructive" size="lg" onClick={() => setConfirm('disable')}>
                  <Power /> Turn off
                </Button>
              )}
            </>
          )
        }
      />

      <div className="mt-7 grid grid-cols-[minmax(0,1fr)_minmax(300px,400px)] gap-x-10 max-[1000px]:grid-cols-1">
        <div className="min-w-0">
        <Section
          title="Settings"
          desc={c.builtin ? 'The console client is managed by Hayat from HAYAT_CONSOLE_URL.' : 'Changes apply to the next sign-in.'}
        >
          <div className="rounded-xl border p-5">
            {c.builtin ? (
              <DetailList
                items={[
                  ['Redirect URIs', <span key="r" className="font-mono text-[12px]">{c.redirectUris.join(', ')}</span>],
                  ['Grants', c.grantTypes.join(', ')],
                ]}
              />
            ) : (
              <ClientForm
                key={c.updatedAt?.seconds.toString()}
                initial={{ name: c.name, kind: c.kind, redirectUris: c.redirectUris, postLogoutRedirectUris: c.postLogoutRedirectUris, grantTypes: c.grantTypes }}
                kindLocked
                submitLabel="Save changes"
                onSubmit={async (v) => {
                  try {
                    await clients.updateClient({ id, name: v.name, redirectUris: v.redirectUris, postLogoutRedirectUris: v.postLogoutRedirectUris, grantTypes: v.grantTypes })
                    toast.success('Client saved')
                    await reload()
                  } catch (err) {
                    toast.error(errorMessage(err))
                  }
                }}
              />
            )}
          </div>
        </Section>
        </div>
        <div className="min-w-0 max-[1000px]:mt-9">
        <Section title="Connect an application" desc="What the application's OIDC library needs.">
          <div className="rounded-xl border p-4">
            <DetailList
              stacked
              items={[
                ['Client id', <CopyField key="id" value={c.id} />],
                ['Issuer', <CopyField key="iss" value={config.issuer} />],
                ['Discovery', <CopyField key="d" value={`${config.issuer}/.well-known/openid-configuration`} />],
                ['Auth method', c.kind === 'public' ? 'None, with PKCE (S256)' : 'client_secret_basic'],
                ['Registered', formatDateTime(c.createdAt)],
              ]}
            />
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
          const res = await clients.rotateClientSecret({ id })
          setSecret(res.secret)
        }}
      />
      <ConfirmDialog
        open={confirm === 'disable'}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={`Turn off ${c.name}?`}
        body="Nobody can sign in to it, and its refresh tokens stop working, until it's turned on again."
        confirm="Turn off"
        destructive
        onConfirm={async () => {
          await clients.setClientDisabled({ id, disabled: true })
          toast.success('Client turned off')
          await reload()
        }}
      />
      {secret && <SecretDialog clientId={c.id} secret={secret} onClose={() => setSecret('')} />}
    </>
  )
}
