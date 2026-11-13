import type { Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { cn } from '@/shared/lib/utils'
import { plural, sourceOf, type ServiceMeta } from '../../lib/roles'

/**
 * Start a new role from a copy of an existing one, or blank. The copy is the
 * admin's own: it won't follow the original when that changes.
 */
export function StartFrom({ roles, services, value, onPick }: { roles: Role[]; services: ServiceMeta[]; value: string; onPick: (role: Role | null) => void }) {
  const meta = (r: Role) => {
    const src = sourceOf(r)
    const who = src === 'builtin' ? 'Built-in' : src === 'custom' ? 'Yours' : (services.find((s) => s.id === r.service)?.name ?? r.service)
    return `${who} · ${plural(r.permissions.length, 'permission')}`
  }
  const option = (id: string, name: string, sub: string, pick: () => void) => (
    <button
      key={id}
      type="button"
      aria-pressed={value === id}
      onClick={pick}
      className={cn(
        'flex flex-col items-start gap-0.5 rounded-[10px] border px-3 py-2 text-left transition-colors',
        value === id ? 'border-primary bg-primary/5 ring-3 ring-primary/12' : 'hover:bg-accent/50',
      )}
    >
      <span className="text-[13px] font-semibold">{name}</span>
      <span className="text-[11.5px] text-muted-foreground">{sub}</span>
    </button>
  )
  return (
    <section aria-labelledby="start-title" className="flex flex-col gap-3 rounded-2xl border bg-background px-5 py-[18px]">
      <div>
        <h2 id="start-title" className="text-[14px] font-semibold">
          Start from
        </h2>
        <p className="mt-0.5 text-[12.5px] text-muted-foreground">Copy an existing role and adjust it, or start blank. The copy is yours: it won't change when the original does.</p>
      </div>
      <div className="flex flex-wrap gap-2">
        {option('', 'Blank', 'No permissions', () => onPick(null))}
        {roles.map((r) => option(r.id, r.name, meta(r), () => onPick(r)))}
      </div>
    </section>
  )
}
