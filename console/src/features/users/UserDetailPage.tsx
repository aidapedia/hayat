import { useState } from 'react'
import { Link, useParams } from 'react-router'
import { toast } from 'sonner'
import { ChevronDown, LogOut, Mail, ShieldCheck, ShieldOff, UserCheck, UserX } from 'lucide-react'
import { UserService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDateTime } from '@/shared/lib/datetime'
import { useMe } from '@/features/auth'
import { ProfileForm, SessionsTable } from '@/features/account'
import { CopyField, DetailList, ErrorState, LoadingState, PersonAvatar, Pill, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/shared/ui/dropdown-menu'
import { PageHeader, Section } from '@/shared/ui/page/PageHeader'

type Pending = { title: string; body: string; confirm: string; destructive?: boolean; run: () => Promise<void> }

export default function UserDetailPage() {
  const { id = '' } = useParams()
  const users = useClient(UserService)
  const { me } = useMe()
  const { data, error, loading, reload } = useQuery(() => users.getUser({ id }), [users, id])
  const [pending, setPending] = useState<Pending | null>(null)

  if (error) return <ErrorState message={error} onRetry={reload} />
  if (loading && !data) return <LoadingState />
  if (!data?.user) return null
  const u = data.user
  const self = u.id === me.user?.id

  const act = (p: Pending) => setPending(p)
  const quick = async (fn: () => Promise<unknown>, done: string) => {
    try {
      await fn()
      toast.success(done)
      await reload()
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }

  return (
    <>
      <PageHeader
        title={u.name || u.email}
        desc={u.name ? u.email : undefined}
        crumbs={[{ label: u.name || u.email }]}
        media={<PersonAvatar name={u.name} email={u.email} size={44} />}
        meta={
          <>
            <StatusBadge status={u.status} />
            {u.isAdmin && <Pill className="border-primary/30 text-primary">Admin</Pill>}
          </>
        }
        action={
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="lg">
                Actions <ChevronDown />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-60">
              {u.status === 'pending' && (
                <DropdownMenuItem className="text-[13px]" onSelect={() => quick(() => users.resendVerification({ id }), 'A new link is on its way')}>
                  <Mail /> Resend the set-up link
                </DropdownMenuItem>
              )}
              {u.status === 'active' &&
                (u.isAdmin ? (
                  <DropdownMenuItem
                    className="text-[13px]"
                    onSelect={() =>
                      act({
                        title: 'Remove instance admin?',
                        body: `${u.email} will no longer be able to open this console's admin pages.`,
                        confirm: 'Remove admin',
                        run: () => users.setUserAdmin({ id, admin: false }).then(() => undefined),
                      })
                    }
                  >
                    <ShieldOff /> Remove instance admin
                  </DropdownMenuItem>
                ) : (
                  <DropdownMenuItem
                    className="text-[13px]"
                    onSelect={() =>
                      act({
                        title: 'Make instance admin?',
                        body: `${u.email} will be able to manage every user, tenant, client and key on this instance.`,
                        confirm: 'Make admin',
                        run: () => users.setUserAdmin({ id, admin: true }).then(() => undefined),
                      })
                    }
                  >
                    <ShieldCheck /> Make instance admin
                  </DropdownMenuItem>
                ))}
              <DropdownMenuItem
                className="text-[13px]"
                disabled={data.sessions.length === 0}
                onSelect={() =>
                  act({
                    title: 'Sign out everywhere?',
                    body: `Every browser signed in as ${u.email} is signed out, and applications lose their refresh tokens.`,
                    confirm: 'Sign out everywhere',
                    destructive: true,
                    run: () => users.revokeUserSessions({ id }).then(() => undefined),
                  })
                }
              >
                <LogOut /> Sign out everywhere
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              {u.status === 'disabled' ? (
                <DropdownMenuItem className="text-[13px]" onSelect={() => quick(() => users.setUserDisabled({ id, disabled: false }), 'Account enabled')}>
                  <UserCheck /> Enable account
                </DropdownMenuItem>
              ) : (
                <DropdownMenuItem
                  variant="destructive"
                  className="text-[13px]"
                  disabled={self}
                  onSelect={() =>
                    act({
                      title: 'Disable this account?',
                      body: `${u.email} is signed out everywhere and can't sign in until the account is enabled again.`,
                      confirm: 'Disable account',
                      destructive: true,
                      run: () => users.setUserDisabled({ id, disabled: true }).then(() => undefined),
                    })
                  }
                >
                  <UserX /> Disable account
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        }
      />

      <div className="mt-7 grid grid-cols-[minmax(0,1fr)_minmax(280px,360px)] gap-x-10 max-[1000px]:grid-cols-1">
        <div className="min-w-0">
          <Section title="Profile">
            <ProfileForm
              user={u}
              onSave={async (p) => {
                await users.updateUser({ id, ...p })
                await reload()
              }}
            />
          </Section>
          <Section title="Sessions" desc="Browsers signed in to Hayat as this person.">
            <SessionsTable
              sessions={data.sessions}
              onRevoke={async (s) => {
                await users.revokeUserSessions({ id, sessionId: s.id })
                await reload()
              }}
            />
          </Section>
        </div>
        <div className="min-w-0 max-[1000px]:mt-9">
          <Section title="Details">
            <div className="rounded-xl border p-4">
              <DetailList
                stacked
                items={[
                  ['User id', <CopyField key="id" value={u.id} />],
                  ['Email', u.emailVerified ? 'Verified' : 'Not verified yet'],
                  ['Password', u.hasPassword ? 'Set' : 'None yet'],
                  ['Home region', u.region],
                  ['Joined', formatDateTime(u.createdAt)],
                  ['Updated', formatDateTime(u.updatedAt)],
                ]}
              />
            </div>
          </Section>
          <Section title="Tenants">
            {data.tenants.length === 0 ? (
              <p className="text-[13px] text-faint">Not a member of any tenant.</p>
            ) : (
              <ul className="divide-y rounded-xl border">
                {data.tenants.map((t) => (
                  <li key={t.id} className="flex items-center justify-between gap-3 px-4 py-2.5 text-[13px]">
                    <Link to={`/tenants/${t.id}`} className="min-w-0 truncate font-semibold hover:underline">
                      {t.name}
                    </Link>
                    {t.ownerId === u.id && <Pill>Owner</Pill>}
                  </li>
                ))}
              </ul>
            )}
          </Section>
        </div>
      </div>

      <ConfirmDialog
        open={!!pending}
        onOpenChange={(o) => !o && setPending(null)}
        title={pending?.title ?? ''}
        body={pending?.body ?? ''}
        confirm={pending?.confirm ?? 'Confirm'}
        destructive={pending?.destructive}
        onConfirm={async () => {
          if (!pending) return
          await pending.run()
          toast.success('Done')
          await reload()
        }}
      />
    </>
  )
}
