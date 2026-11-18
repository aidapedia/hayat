import { useMemo } from 'react'
import type { Member } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { AgentService } from '@/shared/api/gen/hayat/admin/v1/agents_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { Combobox, type ComboboxOption } from '@/shared/ui/combobox'

export type PrincipalKind = 'user' | 'group' | 'agent'

const KINDS: ComboboxOption[] = [
  { value: 'user', label: 'Person' },
  { value: 'group', label: 'Group' },
  { value: 'agent', label: 'Agent' },
]

/** A tenant member as a list row: their name, with the email beside it. */
export const memberOption = (m: Member): ComboboxOption => ({
  value: m.user!.id,
  label: m.user!.name || m.user!.email,
  hint: m.user!.name ? m.user!.email : undefined,
})

/** The tenant's agents as options. Hayat refuses a role only people may hold. */
function useAgentOptions(tenantId: string) {
  const agents = useClient(AgentService)
  const { data } = useQuery(() => (tenantId ? agents.listAgents({ tenantId }) : Promise.resolve(null)), [agents, tenantId])
  return useMemo<ComboboxOption[]>(
    () => (data?.agents ?? []).map((a) => ({ value: a.id, label: a.name, hint: `@${a.handle}`, keywords: [a.handle] })),
    [data],
  )
}

const PLACEHOLDER: Record<PrincipalKind, [string, string]> = {
  user: ['Choose a member', 'Search name or email…'],
  group: ['Choose a group', 'Search groups…'],
  agent: ['Choose an agent', 'Search agents…'],
}

/** Who a grant goes to: a kind (person, group or agent), then which one. */
export function PrincipalPicker({
  tenantId,
  kind,
  principal,
  members,
  groups,
  disabled,
  onKind,
  onPrincipal,
}: {
  tenantId: string
  kind: PrincipalKind
  principal: string
  members: Member[]
  groups: ComboboxOption[]
  disabled?: boolean
  onKind: (k: PrincipalKind) => void
  onPrincipal: (id: string) => void
}) {
  const agents = useAgentOptions(tenantId)
  const options = kind === 'user' ? members.map(memberOption) : kind === 'group' ? groups : agents
  return (
    <div className="flex gap-2">
      <Combobox
        aria-label="Grant to a person, a group or an agent"
        value={kind}
        onChange={(v) => {
          onKind(v as PrincipalKind)
          onPrincipal('')
        }}
        options={KINDS}
        className="w-28 shrink-0"
      />
      <Combobox
        aria-label={kind === 'user' ? 'Member' : kind === 'group' ? 'Group' : 'Agent'}
        value={principal}
        onChange={onPrincipal}
        options={options}
        placeholder={PLACEHOLDER[kind][0]}
        searchPlaceholder={PLACEHOLDER[kind][1]}
        empty={kind === 'agent' ? 'No agents in this tenant.' : undefined}
        disabled={disabled}
        className="flex-1"
        contentClassName="min-w-72"
      />
    </div>
  )
}
