import { useState, type FormEvent } from 'react'
import { Checkbox } from '@/shared/ui/checkbox'
import { Field } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Textarea } from '@/shared/ui/textarea'

export type ClientValues = {
  name: string
  kind: string
  redirectUris: string[]
  postLogoutRedirectUris: string[]
  grantTypes: string[]
}

const GRANTS = [
  { id: 'authorization_code', label: 'Sign-in (authorization code)', hint: 'People sign in through Hayat and come back with a code.' },
  { id: 'refresh_token', label: 'Refresh tokens', hint: 'Stay signed in without sending people back to Hayat.' },
  { id: 'client_credentials', label: 'Client credentials', hint: 'A server calls APIs as itself, with no person involved.' },
]

const lines = (s: string) =>
  s
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)

export function ClientForm({
  initial,
  kindLocked,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  initial: ClientValues
  kindLocked?: boolean
  submitLabel: string
  onSubmit: (v: ClientValues) => Promise<void>
  onCancel?: () => void
}) {
  const [name, setName] = useState(initial.name)
  const [kind, setKind] = useState(initial.kind)
  const [redirects, setRedirects] = useState(initial.redirectUris.join('\n'))
  const [postLogout, setPostLogout] = useState(initial.postLogoutRedirectUris.join('\n'))
  const [grants, setGrants] = useState<string[]>(initial.grantTypes)
  const [busy, setBusy] = useState(false)

  const toggle = (id: string) => setGrants((g) => (g.includes(id) ? g.filter((x) => x !== id) : [...g, id]))

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await onSubmit({ name, kind, redirectUris: lines(redirects), postLogoutRedirectUris: lines(postLogout), grantTypes: grants })
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <div className="grid grid-cols-2 gap-4 max-[640px]:grid-cols-1">
        <Field label="Name" htmlFor="c-name" hint="Shown on the sign-in page: “to continue to …”.">
          <Input id="c-name" value={name} onChange={(e) => setName(e.target.value)} required maxLength={100} />
        </Field>
        <Field label="Kind" htmlFor="c-kind" hint={kind === 'public' ? 'A browser or mobile app. No secret; PKCE is required.' : 'A server that can keep a secret.'}>
          <Select value={kind} onValueChange={setKind} disabled={kindLocked}>
            <SelectTrigger id="c-kind" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="confidential">Confidential (server)</SelectItem>
              <SelectItem value="public">Public (browser or mobile)</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </div>
      <Field label="Redirect URIs" htmlFor="c-redirects" hint="One per line. https, or http only for localhost.">
        <Textarea id="c-redirects" value={redirects} onChange={(e) => setRedirects(e.target.value)} rows={3} className="font-mono text-[12px]" placeholder="https://app.example.com/callback" />
      </Field>
      <Field label="After sign-out, allow returning to" htmlFor="c-logout" hint="Optional. One per line.">
        <Textarea id="c-logout" value={postLogout} onChange={(e) => setPostLogout(e.target.value)} rows={2} className="font-mono text-[12px]" placeholder="https://app.example.com/" />
      </Field>
      <div className="flex flex-col gap-2">
        <span className="text-[12px] font-semibold text-muted-foreground">Grant types</span>
        {GRANTS.map((g) => {
          const off = g.id === 'client_credentials' && kind === 'public'
          return (
            <Label key={g.id} className={`flex items-start gap-3 rounded-[10px] border px-3 py-2.5 font-normal ${off ? 'opacity-50' : 'cursor-pointer hover:bg-accent/50'}`}>
              <Checkbox checked={grants.includes(g.id)} onCheckedChange={() => toggle(g.id)} disabled={off} className="mt-0.5" />
              <span>
                <span className="block text-[13px] font-medium">{g.label}</span>
                <span className="block text-[12px] text-faint">{g.hint}</span>
              </span>
            </Label>
          )
        })}
      </div>
      <div className="flex justify-end gap-2">
        {onCancel && (
          <Button type="button" variant="outline" onClick={onCancel}>
            Cancel
          </Button>
        )}
        <Button type="submit" disabled={busy || !name || grants.length === 0}>
          {busy ? 'Saving…' : submitLabel}
        </Button>
      </div>
    </form>
  )
}
