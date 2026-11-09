import { useMemo, useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import type { User } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage } from '@/shared/api/client'
import { Field } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Input } from '@/shared/ui/input'

export type ProfileInput = { name: string; locale: string; zoneinfo: string; phone: string }

const zones = (() => {
  try {
    return (Intl as unknown as { supportedValuesOf(k: string): string[] }).supportedValuesOf('timeZone')
  } catch {
    return [] as string[]
  }
})()

/** The standard profile: what Hayat hands applications as OIDC claims. */
export function ProfileForm({ user, onSave }: { user: User; onSave: (p: ProfileInput) => Promise<void> }) {
  const initial = useMemo(() => ({ name: user.name, locale: user.locale, zoneinfo: user.zoneinfo, phone: user.phone }), [user])
  const [form, setForm] = useState<ProfileInput>(initial)
  const [busy, setBusy] = useState(false)
  const dirty = JSON.stringify(form) !== JSON.stringify(initial)
  const set = (k: keyof ProfileInput) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value })

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await onSave(form)
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
        <Input id="p-name" value={form.name} onChange={set('name')} maxLength={200} autoComplete="name" />
      </Field>
      <Field label="Phone" htmlFor="p-phone">
        <Input id="p-phone" value={form.phone} onChange={set('phone')} maxLength={40} autoComplete="tel" />
      </Field>
      <Field label="Time zone" htmlFor="p-zone" hint="Times in the console are shown in this zone.">
        <Input id="p-zone" value={form.zoneinfo} onChange={set('zoneinfo')} list="p-zones" placeholder="e.g. Asia/Jakarta" />
        <datalist id="p-zones">
          {zones.map((z) => (
            <option key={z} value={z} />
          ))}
        </datalist>
      </Field>
      <Field label="Language" htmlFor="p-locale" hint="A language tag, such as en or id-ID.">
        <Input id="p-locale" value={form.locale} onChange={set('locale')} maxLength={35} placeholder="en" />
      </Field>
      <div className="col-span-full flex justify-end">
        <Button type="submit" disabled={!dirty || busy}>
          {busy ? 'Saving…' : 'Save profile'}
        </Button>
      </div>
    </form>
  )
}
