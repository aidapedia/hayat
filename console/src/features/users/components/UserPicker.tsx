import { useState } from 'react'
import { ChevronsUpDownIcon, UserPlus } from 'lucide-react'
import { UserService, type User } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { useClient } from '@/shared/api/client'
import { useDebounced } from '@/shared/hooks/use-debounced'
import { useQuery } from '@/shared/hooks/use-query'
import { cn } from '@/shared/lib/utils'
import { PersonRow, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Command, CommandGroup, CommandInput, CommandItem, CommandList } from '@/shared/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/shared/ui/popover'

/** Who was picked: an existing account, or an address with none yet, to be invited. */
export type UserChoice = { kind: 'user'; user: User } | { kind: 'invite'; email: string }

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/** The picked person, the way they appear in every list; an invite shows its address and a pill. */
function ChoiceRow({ choice }: { choice: UserChoice }) {
  if (choice.kind === 'invite') return <PersonRow email={choice.email} trailing={<Pill className="border-warning/30 text-warning">Invite</Pill>} />
  const u = choice.user
  return <PersonRow name={u.name} email={u.email} src={u.avatarUrl} trailing={u.status === 'pending' ? <Pill className="border-warning/30 text-warning">Invited</Pill> : undefined} />
}

/**
 * Pick one person from Hayat's accounts, searched by name or email on the
 * server. An address nobody has an account for is offered as an invitation;
 * the caller sends `user.id` or `email` and the server invites the latter.
 * A disabled account is listed but can't be picked.
 */
export function UserPicker({
  id,
  value,
  onChange,
  placeholder = 'Choose a user',
}: {
  id?: string
  value: UserChoice | null
  onChange: (choice: UserChoice) => void
  placeholder?: string
}) {
  const users = useClient(UserService)
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const q = useDebounced(search.trim())
  const { data, loading } = useQuery(() => (open ? users.listUsers({ query: q, pageSize: 8 }) : Promise.resolve(null)), [users, q, open])
  const found = data?.users ?? []

  const typed = search.trim().toLowerCase()
  const settled = !loading && q === search.trim()
  const canInvite = settled && EMAIL.test(typed) && !found.some((u) => u.email.toLowerCase() === typed)

  const pick = (choice: UserChoice) => {
    onChange(choice)
    setOpen(false)
    setSearch('')
  }
  const invite = () => pick({ kind: 'invite', email: typed })
  const pickedId = value?.kind === 'user' ? value.user.id : ''

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          id={id}
          type="button"
          role="combobox"
          aria-expanded={open}
          className="flex min-h-12 w-full min-w-0 items-center justify-between gap-2 rounded-lg border border-input bg-transparent py-1.5 pr-2 pl-2.5 text-left text-sm transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30 dark:hover:bg-input/50"
        >
          {value ? <ChoiceRow choice={value} /> : <span className="text-muted-foreground">{placeholder}</span>}
          <ChevronsUpDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-(--radix-popover-trigger-width) min-w-72 p-0">
        <Command shouldFilter={false}>
          <CommandInput
            placeholder="Search name or email…"
            value={search}
            onValueChange={setSearch}
            onKeyDown={(e) => {
              // Nothing to highlight when nobody matches: Enter sends the invitation.
              if (e.key === 'Enter' && canInvite && found.length === 0) {
                e.preventDefault()
                invite()
              }
            }}
          />
          <CommandList>
            {found.length > 0 && (
              <CommandGroup>
                {found.map((u) => (
                  <CommandItem
                    key={u.id}
                    value={u.id}
                    disabled={u.status === 'disabled'}
                    data-checked={u.id === pickedId}
                    onSelect={() => pick({ kind: 'user', user: u })}
                  >
                    <div className="min-w-0 flex-1">
                      <ChoiceRow choice={{ kind: 'user', user: u }} />
                    </div>
                    {u.status === 'disabled' && <span className="shrink-0 text-[11px] text-faint">Disabled</span>}
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
            {found.length === 0 && !canInvite && (
              <div className="px-3 py-5 text-center text-[13px] text-muted-foreground">
                {!settled ? 'Searching…' : typed ? 'No account matches. Type a full email to invite them.' : 'No accounts yet.'}
              </div>
            )}
          </CommandList>
          {canInvite && (
            <div className={cn('flex items-center gap-2 p-2', found.length > 0 && 'border-t')}>
              <span className="min-w-0 flex-1 truncate text-[12px] text-muted-foreground">
                No account for <span className="font-medium text-foreground">{typed}</span>
              </span>
              <Button type="button" size="sm" onClick={invite}>
                <UserPlus /> Invite
              </Button>
            </div>
          )}
        </Command>
      </PopoverContent>
    </Popover>
  )
}
