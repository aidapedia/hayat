import type { NodeType, PermissionInfo } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { cn } from '@/shared/lib/utils'
import { Checkbox } from '@/shared/ui/checkbox'
import { idleTypes, typeTree, type ServiceMeta } from '../../lib/roles'

/**
 * Limit the node types the role may be granted on, laid out as the tree they
 * form. A ticked type where none of the role's permissions mean anything is
 * flagged: granting there would do nothing.
 */
export function GrantablePicker({
  types,
  services,
  perms,
  value,
  onChange,
}: {
  types: NodeType[]
  services: ServiceMeta[]
  perms: PermissionInfo[]
  value: string[]
  onChange: (appliesTo: string[]) => void
}) {
  const idle = new Set(idleTypes(perms, types, value))
  return (
    <section aria-labelledby="where-title" className="flex flex-col gap-3.5 rounded-2xl border bg-background px-5 py-[18px]">
      <div>
        <h2 id="where-title" className="text-[14px] font-semibold">
          Where can it be granted?
        </h2>
        <p className="mt-0.5 text-[12.5px] text-muted-foreground">
          Limit the nodes an admin can grant this role on. A grant also reaches everything filed below. Leave all unticked to allow any node.
        </p>
      </div>
      <div className="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-2.5">
        {typeTree(types).map(({ type, path }) => {
          const on = value.includes(type.key)
          const owner = services.find((s) => s.id === type.service)
          return (
            <label
              key={type.key}
              className={cn('flex cursor-pointer flex-col gap-1.5 rounded-xl border px-3.5 py-3 transition-colors', on ? 'border-primary bg-primary/5' : 'hover:bg-accent/50')}
            >
              <span className="flex items-center gap-2">
                <Checkbox checked={on} onCheckedChange={() => onChange(on ? value.filter((k) => k !== type.key) : [...value, type.key])} />
                <span className="text-[13px] font-semibold">{path[path.length - 1]}</span>
                <span className="ml-auto text-[11px] text-muted-foreground">{owner?.name ?? type.service}</span>
              </span>
              {type.description && <span className="text-[12px] text-muted-foreground">{type.description}</span>}
              <span className="text-[11.5px] text-muted-foreground">{path.join(' › ')}</span>
              {idle.has(type.key) && <span className="text-[11.5px] font-medium text-warning">None of the chosen permissions apply here.</span>}
            </label>
          )
        })}
      </div>
    </section>
  )
}
