import { useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { toast } from 'sonner'
import { AccountService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { useAuth, useMe } from '@/features/auth'
import { ErrorState, Field, LoadingState, PersonAvatar, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { PasswordInput } from '@/shared/ui/password-input'
import { Narrow, PageHeader, Section } from '@/shared/ui/page/PageHeader'
import { ProfileForm } from './ProfileForm'
import { SessionsTable } from './SessionsTable'

function PasswordForm({ hasPassword }: { hasPassword: boolean }) {
  const account = useClient(AccountService)
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (next !== confirm) {
      toast.error("The two new passwords don't match.")
      return
    }
    setBusy(true)
    try {
      await account.changePassword({ currentPassword: current, newPassword: next })
      setCurrent('')
      setNext('')
      setConfirm('')
      toast.success('Password changed. Your other sessions were signed out.')
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="grid grid-cols-3 gap-4 rounded-xl border p-5 max-[760px]:grid-cols-1">
      {hasPassword && (
        <Field label="Current password" htmlFor="pw-current">
          <PasswordInput id="pw-current" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" required />
        </Field>
      )}
      <Field label="New password" htmlFor="pw-new" hint="At least 10 characters.">
        <PasswordInput id="pw-new" value={next} onChange={(e) => setNext(e.target.value)} autoComplete="new-password" minLength={10} required />
      </Field>
      <Field label="Repeat new password" htmlFor="pw-confirm">
        <PasswordInput id="pw-confirm" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" minLength={10} required />
      </Field>
      <div className="col-span-full flex items-center justify-between gap-3">
        <p className="text-[12px] text-faint">Changing it signs you out everywhere except here.</p>
        <Button type="submit" disabled={busy || !next}>
          {busy ? 'Saving…' : 'Change password'}
        </Button>
      </div>
    </form>
  )
}

export default function AccountPage() {
  const { me, isAdmin, reload } = useMe()
  const { signOut } = useAuth()
  const account = useClient(AccountService)
  const sessions = useQuery(() => account.listMySessions({}).then((r) => r.sessions), [account])
  const user = me.user!

  return (
    <>
      <PageHeader
        title={user.name || user.email}
        desc={user.email}
        media={<PersonAvatar name={user.name} email={user.email} size={44} />}
        meta={isAdmin ? <Pill className="border-primary/30 text-primary">Instance admin</Pill> : undefined}
        action={
          <Button variant="outline" size="lg" onClick={() => void signOut()}>
            Sign out
          </Button>
        }
      />
      <div className="mt-7">
        <Narrow width={880}>
          <Section title="Profile" desc="Applications that sign you in with Hayat receive these details.">
            <ProfileForm
              user={user}
              onSave={async (p) => {
                await account.updateMe(p)
                await reload()
              }}
            />
          </Section>
          <Section title="Password">
            <PasswordForm hasPassword={user.hasPassword} />
          </Section>
          <Section title="Where you're signed in" desc="Each row is a browser holding a Hayat session.">
            {sessions.error ? (
              <ErrorState message={sessions.error} onRetry={sessions.reload} />
            ) : sessions.loading && !sessions.data ? (
              <LoadingState />
            ) : (
              <SessionsTable
                sessions={sessions.data ?? []}
                onRevoke={async (s) => {
                  await account.revokeMySession({ sessionId: s.id })
                  if (s.current) await signOut()
                  else await sessions.reload()
                }}
              />
            )}
          </Section>
          <Section title="Tenants" desc="Organisations you belong to.">
            {me.tenants.length === 0 ? (
              <p className="text-[13px] text-faint">You aren't a member of any tenant yet.</p>
            ) : (
              <ul className="divide-y rounded-xl border">
                {me.tenants.map((t) => (
                  <li key={t.id} className="flex items-center justify-between gap-3 px-4 py-3 text-[13px]">
                    <span className="min-w-0">
                      {isAdmin ? (
                        <Link to={`/tenants/${t.id}`} className="font-semibold hover:underline">
                          {t.name}
                        </Link>
                      ) : (
                        <span className="font-semibold">{t.name}</span>
                      )}
                      <span className="ml-2 font-mono text-[12px] text-faint">{t.slug}</span>
                    </span>
                    {t.ownerId === user.id && <Pill>Owner</Pill>}
                  </li>
                ))}
              </ul>
            )}
          </Section>
        </Narrow>
      </div>
    </>
  )
}
