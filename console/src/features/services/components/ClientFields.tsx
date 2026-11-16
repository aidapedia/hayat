import { Checkbox } from '@/shared/ui/checkbox'
import { Field } from '@/shared/ui/bits'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { UriListInput } from './UriListInput'

export type ClientValues = {
  name: string
  redirectUris: string[]
  postLogoutRedirectUris: string[]
  grantTypes: string[]
}

const REFRESH = 'refresh_token'

/**
 * The fields of a sign-in client both adding and editing one share. Sign-in
 * (authorization_code) is what a sign-in client is for, so it is always on;
 * other grants a client already has are kept as they are.
 */
export function ClientFields({ value: v, onChange, disabled, redirectPlaceholder }: { value: ClientValues; onChange: (v: ClientValues) => void; disabled?: boolean; redirectPlaceholder?: string }) {
  const refresh = v.grantTypes.includes(REFRESH)
  return (
    <div className="flex flex-col gap-5">
      <Field label="Name" htmlFor="client-name" hint="Shown on the sign-in page.">
        <Input id="client-name" value={v.name} onChange={(e) => onChange({ ...v, name: e.target.value })} required maxLength={100} disabled={disabled} />
      </Field>
      <Field label="Redirect URIs" htmlFor="client-redirect" hint="https, or http only for localhost. Each must match exactly.">
        <UriListInput
          id="client-redirect"
          value={v.redirectUris}
          onChange={(redirectUris) => onChange({ ...v, redirectUris })}
          placeholder={redirectPlaceholder ?? 'https://app.example.com/callback'}
          disabled={disabled}
        />
      </Field>
      <Field label="After sign-out, allow returning to" htmlFor="client-logout" hint="Optional.">
        <UriListInput
          id="client-logout"
          value={v.postLogoutRedirectUris}
          onChange={(postLogoutRedirectUris) => onChange({ ...v, postLogoutRedirectUris })}
          placeholder="https://app.example.com/"
          disabled={disabled}
        />
      </Field>
      <div className="flex flex-col gap-2">
        <span className="text-[12px] font-semibold text-muted-foreground">Grants</span>
        <div className="flex items-start gap-3 rounded-[10px] border bg-muted/50 px-3 py-2.5">
          <Checkbox checked disabled className="mt-0.5" aria-label="Sign-in" />
          <span>
            <span className="block text-[13px] font-medium">Sign-in</span>
            <span className="block text-[12px] text-faint">Always on: it's what a sign-in client is for.</span>
          </span>
        </div>
        <Label className={`flex items-start gap-3 rounded-[10px] border px-3 py-2.5 font-normal ${disabled ? 'opacity-60' : 'cursor-pointer hover:bg-accent/50'}`}>
          <Checkbox
            checked={refresh}
            disabled={disabled}
            onCheckedChange={(on) => onChange({ ...v, grantTypes: on ? [...v.grantTypes, REFRESH] : v.grantTypes.filter((g) => g !== REFRESH) })}
            className="mt-0.5"
          />
          <span>
            <span className="block text-[13px] font-medium">Refresh tokens</span>
            <span className="block text-[12px] text-faint">Stay signed in without returning to Hayat. Rotated on every use.</span>
          </span>
        </Label>
        <p className="text-[12px] text-faint">Server-to-server calls use the service's API credential, not a client here.</p>
      </div>
    </div>
  )
}
