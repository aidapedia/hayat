import { useMemo, useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import type { User } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage } from '@/shared/api/client'
import { Field } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Combobox, type ComboboxOption } from '@/shared/ui/combobox'
import { Input } from '@/shared/ui/input'
import { countries, countryOf, isE164, joinPhone, splitPhone } from './lib/phone'
import { languageOptions, timeZoneOptions } from './lib/profile-options'

export type ProfileInput = { name: string; locale: string; zoneinfo: string; phone: string }

type FormState = { name: string; locale: string; zoneinfo: string; country: string; national: string }

const countryOptions: ComboboxOption[] = countries.map((c) => ({
  value: c.iso,
  label: c.name,
  hint: `+${c.code}`,
  keywords: [c.iso, c.code],
  icon: <span aria-hidden>{c.flag}</span>,
}))

const toState = (user: User): FormState => ({
  name: user.name,
  locale: user.locale,
  zoneinfo: user.zoneinfo,
  ...splitPhone(user.phone),
})

/** The standard profile: what Hayat hands applications as OIDC claims. */
export function ProfileForm({ user, onSave }: { user: User; onSave: (p: ProfileInput) => Promise<void> }) {
  const initial = useMemo(() => toState(user), [user])
  const [form, setForm] = useState<FormState>(initial)
  const [busy, setBusy] = useState(false)
  const zones = useMemo(timeZoneOptions, [])
  const languages = useMemo(() => languageOptions(user.locale), [user.locale])

  const phone = joinPhone(form.country, form.national)
  const phoneInvalid = phone !== '' && !isE164(phone)
  const dirty =
    form.name !== user.name || form.locale !== user.locale || form.zoneinfo !== user.zoneinfo || phone !== user.phone
  const set = <K extends keyof FormState>(k: K) => (v: FormState[K]) => setForm((f) => ({ ...f, [k]: v }))

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (phoneInvalid) return
    setBusy(true)
    try {
      await onSave({ name: form.name, locale: form.locale, zoneinfo: form.zoneinfo, phone })
      toast.success('Profile saved')
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="grid grid-cols-2 gap-4 rounded-xl border p-5 max-[640px]:grid-cols-1">
      <Field label="Name" htmlFor="p-name">
        <Input id="p-name" value={form.name} onChange={(e) => set('name')(e.target.value)} maxLength={200} autoComplete="name" />
      </Field>
      <Field
        label="Phone"
        htmlFor="p-phone"
        hint={phoneInvalid ? <span className="text-destructive">With its country code, a number has 7 to 15 digits.</span> : undefined}
      >
        <div className="flex gap-2">
          <Combobox
            aria-label="Country code"
            value={form.country}
            onChange={set('country')}
            options={countryOptions}
            searchPlaceholder="Search country or code…"
            renderValue={(o) => (
              <>
                {o.icon}
                <span className="tabular-nums">+{countryOf(o.value)?.code}</span>
              </>
            )}
            className="w-26 shrink-0"
            contentClassName="w-72"
          />
          <Input
            id="p-phone"
            type="tel"
            inputMode="tel"
            value={form.national}
            onChange={(e) => set('national')(e.target.value.replace(/[^\d\s\-()]/g, ''))}
            maxLength={20}
            autoComplete="tel-national"
            placeholder="812 3456 7890"
            aria-invalid={phoneInvalid || undefined}
          />
        </div>
      </Field>
      <Field label="Time zone" htmlFor="p-zone" hint="Times in the console are shown in this zone.">
        <Combobox
          id="p-zone"
          value={form.zoneinfo}
          onChange={set('zoneinfo')}
          options={zones}
          placeholder="Not set"
          searchPlaceholder="Search city or offset…"
          clearLabel="Not set"
          renderValue={(o) => (
            <>
              <span className="truncate">{o.label}</span>
              <span className="text-muted-foreground">{o.hint}</span>
            </>
          )}
        />
      </Field>
      <Field label="Language" htmlFor="p-locale" hint="Applications use it to pick their translation.">
        <Combobox
          id="p-locale"
          value={form.locale}
          onChange={set('locale')}
          options={languages}
          placeholder="Not set"
          searchPlaceholder="Search language…"
          clearLabel="Not set"
        />
      </Field>
      <div className="col-span-full flex justify-end">
        <Button type="submit" disabled={!dirty || busy || phoneInvalid}>
          {busy ? 'Saving…' : 'Save profile'}
        </Button>
      </div>
    </form>
  )
}
