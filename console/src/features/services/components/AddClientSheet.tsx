import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { AppWindow, Server, SquareTerminal } from 'lucide-react'
import { RegistryService, type ServiceInfo } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { cn } from '@/shared/lib/utils'
import { Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/shared/ui/sheet'
import { ClientFields, type ClientValues } from './ClientFields'
import { SecretDialog } from './SecretDialog'
import { ServiceTile } from './ServiceTile'

type AppType = 'web' | 'spa' | 'native'

const TYPES: { id: AppType; label: string; hint: string; Icon: typeof Server; kind: 'public' | 'confidential'; auth: string; note: string; redirect: string }[] = [
  {
    id: 'web',
    label: 'Web server app',
    hint: 'Renders on a server that can keep a secret.',
    Icon: Server,
    kind: 'confidential',
    auth: 'client_secret_basic',
    note: 'Gets a secret, shown once.',
    redirect: 'https://app.example.com/auth/callback',
  },
  {
    id: 'spa',
    label: 'Single-page app',
    hint: 'Runs in the browser: React, Vue, Svelte.',
    Icon: AppWindow,
    kind: 'public',
    auth: 'PKCE · S256',
    note: 'No secret: a browser can’t keep one.',
    redirect: 'https://console.example.com/callback',
  },
  {
    id: 'native',
    label: 'Native or CLI',
    hint: 'Desktop, mobile or terminal app.',
    Icon: SquareTerminal,
    kind: 'public',
    auth: 'PKCE · S256',
    note: 'No secret: anyone can unpack the app.',
    redirect: 'http://127.0.0.1/callback',
  },
]

/** Adding a sign-in client to a service: what kind of app, then its fields. */
export function AddClientSheet({ service, open, onOpenChange, onAdded }: { service: ServiceInfo; open: boolean; onOpenChange: (o: boolean) => void; onAdded: () => unknown }) {
  const registry = useClient(RegistryService)
  const navigate = useNavigate()
  const [type, setType] = useState<AppType>('spa')
  const [values, setValues] = useState<ClientValues>({ name: '', redirectUris: [], postLogoutRedirectUris: [], grantTypes: ['authorization_code', 'refresh_token'] })
  const [busy, setBusy] = useState(false)
  const [created, setCreated] = useState<{ id: string; secret: string } | null>(null)
  const t = TYPES.find((x) => x.id === type)!

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const res = await registry.createServiceClient({ serviceId: service.id, kind: t.kind, ...values })
      const c = res.client!
      toast.success(`${c.name} added`)
      await onAdded()
      onOpenChange(false)
      setValues({ name: '', redirectUris: [], postLogoutRedirectUris: [], grantTypes: ['authorization_code', 'refresh_token'] })
      if (res.secret) setCreated({ id: c.id, secret: res.secret })
      else navigate(`/services/${service.id}/clients/${c.id}`)
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent className="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-[720px]">
          <form onSubmit={submit} className="flex h-full flex-col">
            <SheetHeader className="flex-row items-start gap-3 border-b">
              <ServiceTile name={service.displayName} builtin={service.builtin} size={36} />
              <div className="flex flex-col gap-1">
                <SheetTitle className="text-[15px]">Add a sign-in client to {service.displayName}</SheetTitle>
                <SheetDescription className="text-[12px]">An app people sign in through. It belongs to this service, and its owners manage it.</SheetDescription>
              </div>
            </SheetHeader>

            <div className="flex flex-1 flex-col gap-6 overflow-y-auto px-4 py-5">
              <fieldset className="flex flex-col gap-2.5">
                <legend className="mb-2 text-[12px] font-semibold text-muted-foreground">What kind of app is it?</legend>
                <div className="grid grid-cols-3 gap-2.5 max-[560px]:grid-cols-1">
                  {TYPES.map((x) => (
                    <button
                      key={x.id}
                      type="button"
                      aria-pressed={type === x.id}
                      onClick={() => setType(x.id)}
                      className={cn(
                        'flex flex-col items-start gap-2 rounded-xl border p-3 text-left transition-colors hover:border-primary/50',
                        type === x.id && 'border-primary bg-primary/5 ring-1 ring-primary',
                      )}
                    >
                      <x.Icon className="size-[18px] text-primary" />
                      <span className="text-[13px] font-semibold">{x.label}</span>
                      <span className="text-[11px] leading-snug text-muted-foreground">{x.hint}</span>
                    </button>
                  ))}
                </div>
                <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                  <Pill>{t.kind}</Pill>
                  <Pill className="font-mono normal-case">{t.auth}</Pill>
                  {t.note}
                </div>
              </fieldset>
              <ClientFields value={values} onChange={setValues} redirectPlaceholder={t.redirect} />
            </div>

            <SheetFooter className="flex-row items-center justify-between border-t">
              <span className="text-[11px] text-faint">{t.kind === 'confidential' ? 'The secret is shown once, right after you add it.' : 'The app needs only the client id.'}</span>
              <div className="flex gap-2">
                <Button type="button" size="lg" variant="outline" onClick={() => onOpenChange(false)}>
                  Cancel
                </Button>
                <Button type="submit" size="lg" disabled={busy || !values.name.trim() || values.redirectUris.length === 0}>
                  {busy ? 'Adding…' : 'Add client'}
                </Button>
              </div>
            </SheetFooter>
          </form>
        </SheetContent>
      </Sheet>
      {created && (
        <SecretDialog
          clientId={created.id}
          secret={created.secret}
          onClose={() => {
            const id = created.id
            setCreated(null)
            navigate(`/services/${service.id}/clients/${id}`)
          }}
        />
      )}
    </>
  )
}
