import { useState, type FormEvent } from 'react'
import { Globe, Plus, ShieldCheck, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { SignInService, type SignInProvider, type TenantDomain } from '@/shared/api/gen/hayat/admin/v1/sign_in_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { CopyField, EmptyState, ErrorState, Field, LoadingState, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Combobox } from '@/shared/ui/combobox'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'
import { PasswordInput } from '@/shared/ui/password-input'
import { Switch } from '@/shared/ui/switch'
import { ProviderIcon } from './ProviderIcon'

const POLICIES = [
  { value: 'offer', label: 'Offered beside the password' },
  { value: 'enforce', label: 'The only way in' },
]

function ProviderDialog({
  tenantId,
  current,
  callbackUrl,
  onClose,
  onSaved,
}: {
  tenantId: string
  current?: SignInProvider
  callbackUrl: string
  onClose: () => void
  onSaved: () => void
}) {
  const signIn = useClient(SignInService)
  const [name, setName] = useState(current?.name ?? '')
  const [issuer, setIssuer] = useState(current?.issuer ?? '')
  const [clientId, setClientId] = useState(current?.clientId ?? '')
  const [secret, setSecret] = useState('')
  const [enabled, setEnabled] = useState(current?.enabled ?? true)
  const [jit, setJit] = useState(current?.jit ?? false)
  const [busy, setBusy] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await signIn.saveTenantProvider({ tenantId, providerId: current?.id ?? '', name, issuer, clientId, clientSecret: secret, enabled, jit })
      toast.success(`${name} saved`)
      onSaved()
      onClose()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">{current ? `Edit ${current.name}` : 'Connect your identity provider'}</DialogTitle>
            <DialogDescription className="text-[13px]">
              Any OpenID Connect provider: Okta, Entra ID, Google Workspace, Keycloak. Create a web app there with this callback URL and the scopes openid, email and profile.
            </DialogDescription>
          </DialogHeader>
          <CopyField value={callbackUrl} />
          <div className="grid grid-cols-2 gap-4 max-[560px]:grid-cols-1">
            <Field label="Name on the button" htmlFor="tp-name">
              <Input id="tp-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Acme SSO" maxLength={60} required autoFocus />
            </Field>
            <Field label="Issuer" htmlFor="tp-issuer">
              <Input id="tp-issuer" value={issuer} onChange={(e) => setIssuer(e.target.value)} placeholder="https://acme.okta.com" required />
            </Field>
            <Field label="Client ID" htmlFor="tp-id">
              <Input id="tp-id" value={clientId} onChange={(e) => setClientId(e.target.value)} autoComplete="off" required />
            </Field>
            <Field label="Client secret" htmlFor="tp-secret" hint={current?.hasSecret ? 'Saved. Leave empty to keep it.' : undefined}>
              <PasswordInput id="tp-secret" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={current?.hasSecret ? '••••••••••••' : ''} autoComplete="new-password" />
            </Field>
          </div>
          <label className="flex items-start justify-between gap-3">
            <span>
              <span className="block text-[13px] font-medium">Turned on</span>
              <span className="block text-[12px] text-muted-foreground">Off stops every sign-in through it.</span>
            </span>
            <Switch checked={enabled} onCheckedChange={setEnabled} />
          </label>
          <label className="flex items-start justify-between gap-3">
            <span>
              <span className="block text-[13px] font-medium">Create accounts for new people</span>
              <span className="block text-[12px] text-muted-foreground">
                Someone at a verified domain without a Hayat account gets one on their first sign-in and joins this tenant. Off: they need an invite first.
              </span>
            </span>
            <Switch checked={jit} onCheckedChange={setJit} />
          </label>
          <DialogFooter className="sm:justify-between">
            {current ? (
              <Button type="button" variant="ghost" className="text-destructive" onClick={() => setDeleting(true)}>
                <Trash2 /> Disconnect
              </Button>
            ) : (
              <span />
            )}
            <div className="flex gap-2">
              <Button type="button" variant="outline" onClick={onClose}>
                Cancel
              </Button>
              <Button type="submit" disabled={busy || !name || !issuer || !clientId || (!current?.hasSecret && !secret)}>
                {busy ? 'Saving…' : current ? 'Save' : 'Connect'}
              </Button>
            </div>
          </DialogFooter>
        </form>
        {deleting && current && (
          <ConfirmDialog
            open
            onOpenChange={(o) => !o && setDeleting(false)}
            title={`Disconnect ${current.name}?`}
            body="Nobody can sign in through it any more. Accounts stay, and people can still use their password or another way in."
            confirm="Disconnect"
            destructive
            onConfirm={async () => {
              await signIn.deleteTenantProvider({ tenantId, providerId: current.id })
              toast.success(`${current.name} disconnected`)
              onSaved()
              onClose()
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function DomainRow({ tenantId, domain: d, providers, onChanged }: { tenantId: string; domain: TenantDomain; providers: SignInProvider[]; onChanged: () => void }) {
  const signIn = useClient(SignInService)
  const [busy, setBusy] = useState(false)
  const [removing, setRemoving] = useState(false)

  const run = async (fn: () => Promise<unknown>, done?: string) => {
    setBusy(true)
    try {
      await fn()
      if (done) toast.success(done)
      onChanged()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-3 border-t px-4 py-4 first:border-t-0">
      <div className="flex flex-wrap items-center gap-2.5">
        <Globe className="size-4 text-muted-foreground" />
        <span className="font-mono text-[13px] font-semibold">{d.domain}</span>
        {d.verified ? <Pill className="border-success/30 text-success">Verified</Pill> : <Pill className="border-warning/30 text-warning">Waiting for DNS</Pill>}
        {d.policy === 'enforce' && <Pill className="border-primary/30 text-primary">Company sign-in only</Pill>}
        <Button variant="ghost" size="sm" className="ml-auto text-muted-foreground" disabled={busy} onClick={() => setRemoving(true)}>
          <Trash2 /> Remove
        </Button>
      </div>
      {!d.verified ? (
        <div className="flex flex-col gap-2.5">
          <p className="text-[12px] text-muted-foreground">Add this TXT record at your DNS host to prove {d.domain} is yours, then check it. Until then the domain does nothing.</p>
          <div className="grid grid-cols-[110px_minmax(0,1fr)] items-center gap-x-3 gap-y-2 text-[12px] max-[560px]:grid-cols-1">
            <span className="text-muted-foreground">Name</span>
            <CopyField value={d.txtName} />
            <span className="text-muted-foreground">Value</span>
            <CopyField value={d.txtValue} />
          </div>
          <div>
            <Button variant="outline" size="sm" disabled={busy} onClick={() => run(() => signIn.verifyTenantDomain({ tenantId, domain: d.domain }), `${d.domain} verified`)}>
              <ShieldCheck /> Check the record
            </Button>
          </div>
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-3 max-[560px]:grid-cols-1">
          <Field label="People here sign in with">
            <Combobox
              aria-label={`Provider for ${d.domain}`}
              value={d.providerId}
              disabled={busy || providers.length === 0}
              placeholder={providers.length ? 'Choose a provider' : 'Connect a provider first'}
              clearLabel="No company sign-in"
              options={providers.map((p) => ({ value: p.id, label: p.name, hint: p.enabled ? undefined : 'off' }))}
              onChange={(v) => run(() => signIn.setTenantDomainSignIn({ tenantId, domain: d.domain, providerId: v, policy: v ? d.policy : 'offer' }))}
            />
          </Field>
          <Field label="Company sign-in is" hint={d.policy === 'enforce' ? 'No password works for these addresses, except an instance admin’s.' : undefined}>
            <Combobox
              aria-label={`Policy for ${d.domain}`}
              value={d.policy}
              disabled={busy || !d.providerId}
              options={POLICIES}
              onChange={(v) => run(() => signIn.setTenantDomainSignIn({ tenantId, domain: d.domain, providerId: d.providerId, policy: v }), v === 'enforce' ? `${d.domain} now signs in only through its provider` : undefined)}
            />
          </Field>
        </div>
      )}
      {removing && (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setRemoving(false)}
          title={`Remove ${d.domain}?`}
          body="People at this domain stop being sent to your provider; they sign in with a password or another way in. You can add it again, but it needs verifying again."
          confirm="Remove domain"
          destructive
          onConfirm={async () => {
            await signIn.removeTenantDomain({ tenantId, domain: d.domain })
            onChanged()
          }}
        />
      )}
    </div>
  )
}

/**
 * A tenant's company sign-in: its own identity provider, and the email
 * domains it has proved it owns. Shown to whoever holds hayat.tenant.manage.
 */
export function TenantSignIn({ tenantId }: { tenantId: string }) {
  const signIn = useClient(SignInService)
  const { data, error, loading, reload } = useQuery(() => signIn.getTenantSignIn({ tenantId }), [signIn, tenantId])
  const [editing, setEditing] = useState<SignInProvider | 'new' | null>(null)
  const [domain, setDomain] = useState('')
  const [adding, setAdding] = useState(false)

  if (error) return <ErrorState message={error} onRetry={reload} />
  if (loading && !data) return <LoadingState />
  if (!data) return null

  const add = async (e: FormEvent) => {
    e.preventDefault()
    setAdding(true)
    try {
      await signIn.addTenantDomain({ tenantId, domain })
      setDomain('')
      reload()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setAdding(false)
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <div className="rounded-xl border">
        <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
          <div>
            <div className="text-[13px] font-semibold">Identity provider</div>
            <div className="text-[12px] text-muted-foreground">Your company's own sign-in, for the domains below.</div>
          </div>
          <Button variant="outline" size="sm" onClick={() => setEditing('new')}>
            <Plus /> Connect
          </Button>
        </div>
        {data.providers.length === 0 ? (
          <p className="px-4 py-4 text-[12px] text-muted-foreground">
            None yet. People sign in with a password, or with Google or GitHub when the instance offers them.
          </p>
        ) : (
          data.providers.map((p) => (
            <button key={p.id} type="button" onClick={() => setEditing(p)} className="flex w-full items-center gap-3 border-t px-4 py-3 text-left first:border-t-0 hover:bg-muted/50">
              <ProviderIcon kind="oidc" className="size-4 text-muted-foreground" />
              <span className="min-w-0 flex-1">
                <span className="block text-[13px] font-semibold">{p.name}</span>
                <span className="block truncate font-mono text-[12px] text-muted-foreground">{p.issuer}</span>
              </span>
              {p.jit && <Pill>Creates accounts</Pill>}
              {p.enabled ? <Pill className="border-success/30 text-success">On</Pill> : <Pill>Off</Pill>}
            </button>
          ))
        )}
      </div>

      <div className="rounded-xl border">
        <div className="border-b px-4 py-3">
          <div className="text-[13px] font-semibold">Email domains</div>
          <div className="text-[12px] text-muted-foreground">Only addresses at a verified domain can sign in through your provider, and only those can be made to.</div>
        </div>
        {data.domains.length === 0 ? (
          <div className="p-4">
            <EmptyState icon={Globe} title="No domains yet">
              Add your company's email domain, then prove it with a DNS record.
            </EmptyState>
          </div>
        ) : (
          data.domains.map((d) => <DomainRow key={d.domain} tenantId={tenantId} domain={d} providers={data.providers} onChanged={reload} />)
        )}
        <form onSubmit={add} className="flex gap-2 border-t px-4 py-3">
          <Input value={domain} onChange={(e) => setDomain(e.target.value)} placeholder="acme.com" aria-label="Domain to add" className="max-w-xs font-mono" />
          <Button type="submit" variant="outline" disabled={adding || !domain.trim()}>
            {adding ? 'Adding…' : 'Add domain'}
          </Button>
        </form>
      </div>

      {editing && (
        <ProviderDialog
          tenantId={tenantId}
          current={editing === 'new' ? undefined : editing}
          callbackUrl={data.callbackUrl}
          onClose={() => setEditing(null)}
          onSaved={reload}
        />
      )}
    </div>
  )
}
