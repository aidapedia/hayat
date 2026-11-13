import type { NodeType } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { cn } from '@/shared/lib/utils'
import { typeLabel, typeTree } from '../lib/roles'

/**
 * The node types as the tree they form, with the ones a role may be granted
 * on lit. Empty `appliesTo` means any node, so every type is lit.
 */
export function TypeTree({ types, appliesTo }: { types: NodeType[]; appliesTo: string[] }) {
  const any = appliesTo.length === 0
  return (
    <ul className="flex flex-col gap-0.5 text-[12.5px]">
      {typeTree(types).map(({ type, depth }) => {
        const on = any || appliesTo.includes(type.key)
        return (
          <li
            key={type.key}
            style={{ paddingLeft: 8 + depth * 18 }}
            className={cn('flex items-center gap-2 rounded-md py-1 pr-2', on ? 'bg-primary/10 font-semibold text-foreground' : 'text-muted-foreground')}
          >
            <span className={cn('size-2 shrink-0 rounded-full', on ? 'bg-primary' : 'border-[1.5px] border-faint')} />
            <span>{typeLabel(type.key)}</span>
            {on && !any && <span className="ml-auto text-[11px] font-normal text-muted-foreground">grantable</span>}
          </li>
        )
      })}
    </ul>
  )
}
