import type { Member } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { Combobox, type ComboboxOption } from '@/shared/ui/combobox'

export type PrincipalKind = 'user' | 'group'

const KINDS: ComboboxOption[] = [
  { value: 'user', label: 'Person' },
  { value: 'group', label: 'Group' },
]

/** A tenant member as a list row: their name, with the email beside it. */
export const memberOption = (m: Member): ComboboxOption => ({
  value: m.user!.id,
  label: m.user!.name || m.user!.email,
  hint: m.user!.name ? m.user!.email : undefined,
})

/** Who a grant goes to: a kind (person or group), then which one. */
export function PrincipalPicker({
  kind,
  principal,
  members,
  groups,
  disabled,
  onKind,
  onPrincipal,
}: {
  kind: PrincipalKind
  principal: string
  members: Member[]
  groups: ComboboxOption[]
  disabled?: boolean
  onKind: (k: PrincipalKind) => void
  onPrincipal: (id: string) => void
}) {
  return (
    <div className="flex gap-2">
      <Combobox
        aria-label="Grant to a person or a group"
        value={kind}
        onChange={(v) => {
          onKind(v as PrincipalKind)
          onPrincipal('')
        }}
        options={KINDS}
        className="w-28 shrink-0"
      />
      <Combobox
        aria-label={kind === 'user' ? 'Member' : 'Group'}
        value={principal}
        onChange={onPrincipal}
        options={kind === 'user' ? members.map(memberOption) : groups}
        placeholder={kind === 'user' ? 'Choose a member' : 'Choose a group'}
        searchPlaceholder={kind === 'user' ? 'Search name or email…' : 'Search groups…'}
        disabled={disabled}
        className="flex-1"
        contentClassName="min-w-72"
      />
    </div>
  )
}
