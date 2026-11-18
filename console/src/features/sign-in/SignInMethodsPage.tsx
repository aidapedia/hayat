import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { toast } from 'sonner'
import { SignInService, type SignInProvider } from '@/shared/api/gen/hayat/admin/v1/sign_in_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDateTime } from '@/shared/lib/datetime'
import { CopyField, ErrorState, Field, LoadingState, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Input } from '@/shared/ui/input'
import { PasswordInput } from '@/shared/ui/password-input'
import { Narrow, PageHeader, Section } from '@/shared/ui/page/PageHeader'
import { Switch } from '@/shared/ui/switch'
import { ProviderIcon } from './ProviderIcon'

type Kind = 'google' | 'github' | 'oidc'

const KINDS: { kind: Kind; title: string; desc: string; help: ReactNode }[] = [
  {
    kind: 'google',
    title: 'Google',
    desc: 'Anyone with a Google account, and Google Workspace users.',
    help: (
      <>
        In Google Cloud console, open APIs &amp; Services → Credentials → Create credentials → OAuth client ID, type
        &ldquo;Web application&rdquo;. Add the callback URL below as an authorised redirect URI.
      </>
    ),
  },
  {
    kind: 'github',
    title: 'GitHub',
    desc: 'Anyone with a GitHub account. Only an address GitHub verified signs in.',
    help: <>In GitHub, open Settings → Developer settings → OAuth Apps → New OAuth App. Use the callback URL below as the authorization callback URL.</>,
  },
  {
    kind: 'oidc',
    title: 'Company SSO',
    desc: "One OpenID Connect provider for everyone on this instance: your organisation's Okta, Entra ID, Google Workspace or Keycloak.",
    help: (
      <>
        Create an OpenID Connect web app in your identity provider with the callback URL below, and the scopes openid, email and profile.
        Use this when this Hayat serves one organisation; a tenant that brings its own provider connects it on its tenant page instead.
      </>
    ),
  },
]

function ProviderCard({ kind, title, desc, help, current, onSaved }: (typeof KINDS)[number] & { current?: SignInProvider; onSaved: () => void }) {
  const signIn = useClient(SignInService)
  const [name, setName] = useState(current?.name ?? '')
  const [issuer, setIssuer] = useState(current?.issuer ?? '')
  const [clientId, setClientId] = useState(current?.clientId ?? '')
  const [secret, setSecret] = useState('')
  const [enabled, setEnabled] = useState(current?.enabled ?? true)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    setName(current?.name ?? '')
    setIssuer(current?.issuer ?? '')
    setClientId(current?.clientId ?? '')
    setEnabled(current?.enabled ?? true)
  }, [current])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await signIn.saveInstanceProvider({ kind, name, issuer, clientId, clientSecret: secret, enabled })
      setSecret('')
      toast.success(`${kind === 'oidc' ? name || 'Company SSO' : title} saved`)
      onSaved()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const status = !current ? <Pill>Not set up</Pill> : current.enabled ? <Pill className="border-success/30 text-success">On</Pill> : <Pill>Off</Pill>

  return (
    <form onSubmit={submit} className="flex flex-col gap-4 rounded-xl border p-5">
      <div className="flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border bg-card">
          <ProviderIcon kind={kind} />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="text-[14px] font-semibold">{kind === 'oidc' && current?.name ? current.name : title}</h3>
            {status}
          </div>
          <p className="mt-0.5 text-[12px] text-muted-foreground">{desc}</p>
        </div>
        <label className="flex shrink-0 items-center gap-2 text-[12px] text-muted-foreground">
          On the login page
          <Switch checked={enabled} onCheckedChange={setEnabled} aria-label={`Offer ${title}`} />
        </label>
      </div>
      <p className="rounded-lg bg-muted/60 px-3 py-2 text-[12px] text-muted-foreground">{help}</p>
      {kind === 'oidc' && (
        <div className="grid grid-cols-2 gap-4 max-[640px]:grid-cols-1">
          <Field label="Name on the button" htmlFor={`${kind}-name`}>
            <Input id={`${kind}-name`} value={name} onChange={(e) => setName(e.target.value)} placeholder="Company SSO" maxLength={60} />
          </Field>
          <Field label="Issuer" htmlFor={`${kind}-issuer`} hint="Hayat reads its OpenID Connect discovery from here.">
            <Input id={`${kind}-issuer`} value={issuer} onChange={(e) => setIssuer(e.target.value)} placeholder="https://acme.okta.com" />
          </Field>
        </div>
      )}
      <div className="grid grid-cols-2 gap-4 max-[640px]:grid-cols-1">
        <Field label="Client ID" htmlFor={`${kind}-id`}>
          <Input id={`${kind}-id`} value={clientId} onChange={(e) => setClientId(e.target.value)} autoComplete="off" />
        </Field>
        <Field label="Client secret" htmlFor={`${kind}-secret`} hint={current?.hasSecret ? 'Saved and encrypted. Leave empty to keep it.' : undefined}>
          <PasswordInput
            id={`${kind}-secret`}
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
            placeholder={current?.hasSecret ? '••••••••••••' : ''}
            autoComplete="new-password"
          />
        </Field>
      </div>
      <div className="flex items-center justify-between gap-3">
        <span className="text-[12px] text-faint">{current ? `Changed ${formatDateTime(current.updatedAt)}` : ''}</span>
        <Button type="submit" disabled={busy || !clientId || (!current?.hasSecret && !secret)}>
          {busy ? 'Saving…' : current ? 'Save' : `Set up ${title}`}
        </Button>
      </div>
    </form>
  )
}

/** /sign-in-methods — the ways in the instance offers everyone. Instance admins only. */
export default function SignInMethodsPage() {
  const signIn = useClient(SignInService)
  const { data, error, loading, reload } = useQuery(() => signIn.listInstanceProviders({}), [signIn])

  return (
    <>
      <PageHeader
        title="Sign-in methods"
        desc="Besides an email and password, the ways people can sign in to every service that uses Hayat. A tenant can add its own company sign-in on its tenant page."
      />
      <div className="mt-7">
        <Narrow width={880}>
          {error ? (
            <ErrorState message={error} onRetry={reload} />
          ) : loading && !data ? (
            <LoadingState />
          ) : (
            <>
              <Section title="Callback URL" desc="Register this redirect URI with every provider below.">
                <CopyField value={data?.callbackUrl ?? ''} />
              </Section>
              <Section title="Providers">
                <div className="flex flex-col gap-4">
                  {KINDS.map((k) => (
                    <ProviderCard key={k.kind} {...k} current={data?.providers.find((p) => p.kind === k.kind)} onSaved={reload} />
                  ))}
                </div>
              </Section>
            </>
          )}
        </Narrow>
      </div>
    </>
  )
}
