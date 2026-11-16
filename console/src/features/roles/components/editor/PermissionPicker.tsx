import { useState } from 'react'
import { AlertTriangle, ChevronRight } from 'lucide-react'
import { cn } from '@/shared/lib/utils'
import { SearchInput } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Checkbox } from '@/shared/ui/checkbox'
import { PLATFORM, permLabel, takesEffect, typeLabel, type Catalog, type ServiceMeta } from '../../lib/roles'
import { ServiceMark } from '../marks'

/**
 * Pick permissions, grouped by the service that enforces them. A ticked
 * permission that can't take effect where the role may be granted says so on
 * its row, so the warning sits next to the cause.
 */
export function PermissionPicker({
  catalog,
  services,
  value,
  appliesTo,
  onChange,
}: {
  catalog: Catalog
  services: ServiceMeta[]
  value: string[]
  appliesTo: string[]
  onChange: (perms: string[]) => void
}) {
  const [q, setQ] = useState('')
  // Open what's in use; with nothing picked yet, open everything.
  const [open, setOpen] = useState<Record<string, boolean>>(() => {
    const used = new Set(catalog.permissions.filter((p) => value.includes(p.key)).map((p) => p.service))
    return Object.fromEntries(services.map((s) => [s.id, used.size === 0 || used.has(s.id)]))
  })
  const words = q.trim().toLowerCase().split(/\s+/).filter(Boolean)

  const toggle = (key: string) => onChange(value.includes(key) ? value.filter((k) => k !== key) : [...value, key])

  return (
    <section aria-labelledby="perm-title" className="flex flex-col gap-3.5 rounded-2xl border bg-background px-5 py-[18px]">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 id="perm-title" className="text-[14px] font-semibold">
            What can someone with this role do?
          </h2>
          <p className="mt-0.5 text-[12.5px] text-muted-foreground">Permissions are grouped by the service that enforces them. Mix services freely.</p>
        </div>
        <SearchInput value={q} onChange={(e) => setQ(e.target.value)} placeholder="Filter permissions" aria-label="Filter permissions" className="w-[260px] max-w-full" />
      </div>

      {services.map((s) => {
        const all = catalog.permissions.filter((p) => p.service === s.id)
        if (all.length === 0) return null
        const shown = words.length
          ? all.filter((p) => {
              const text = `${p.key} ${permLabel(p)} ${p.description}`.toLowerCase()
              return words.every((w) => text.includes(w))
            })
          : all
        if (words.length && shown.length === 0) return null
        const keys = all.map((p) => p.key)
        const picked = keys.filter((k) => value.includes(k)).length
        const isOpen = words.length > 0 || open[s.id]
        return (
          <div key={s.id} className="overflow-hidden rounded-xl border">
            <div className="flex items-center gap-3 bg-muted/40 px-3.5 py-3">
              <button
                type="button"
                aria-expanded={isOpen}
                onClick={() => setOpen({ ...open, [s.id]: !open[s.id] })}
                className="flex min-w-0 flex-1 items-center gap-3 text-left"
              >
                <ChevronRight className={cn('size-4 shrink-0 text-muted-foreground transition-transform', isOpen && 'rotate-90')} />
                <ServiceMark service={s} size="md" />
                <span className="flex min-w-0 flex-col">
                  <span className="text-[13.5px] font-semibold">
                    {s.name}
                    <span className="font-normal text-muted-foreground"> · {s.id === PLATFORM ? 'Platform' : `Service · contract v${s.version}`}</span>
                  </span>
                  {s.description && <span className="truncate text-[12px] text-muted-foreground">{s.description}</span>}
                </span>
              </button>
              <span className={cn('inline-flex h-[22px] shrink-0 items-center rounded-md px-2 text-[11.5px] font-semibold', picked ? 'bg-primary/12 text-primary' : 'bg-muted text-muted-foreground')}>
                {picked} of {keys.length}
              </span>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => onChange(picked === keys.length ? value.filter((k) => !keys.includes(k)) : [...new Set([...value, ...keys])])}
              >
                {picked === keys.length ? 'Clear' : 'Select all'}
              </Button>
            </div>
            {isOpen && (
              <div className="flex flex-col">
                {shown.map((p) => {
                  const on = value.includes(p.key)
                  const warn = on && !takesEffect(catalog.types, p.scope, appliesTo)
                  return (
                    <label
                      key={p.key}
                      className={cn('flex cursor-pointer items-start gap-3 border-t px-3.5 py-3', warn ? 'bg-warning/8' : on ? 'bg-primary/4' : 'hover:bg-accent/50')}
                    >
                      <Checkbox checked={on} onCheckedChange={() => toggle(p.key)} className="mt-0.5" />
                      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                        <span className="flex flex-wrap items-center gap-2">
                          <span className="text-[13px] font-semibold">{permLabel(p)}</span>
                          <span className="font-mono text-[11px] text-muted-foreground">{p.key}</span>
                        </span>
                        {p.description && <span className="text-[12.5px] text-muted-foreground">{p.description}</span>}
                        {warn && (
                          <span className="mt-1 inline-flex items-center gap-1.5 text-[12px] font-medium text-warning">
                            <AlertTriangle className="size-3.5 shrink-0" />
                            Only takes effect when granted on {typeLabel(p.scope)} or above, and this role can't be granted there yet.
                          </span>
                        )}
                      </span>
                      <span className="inline-flex h-[22px] shrink-0 items-center rounded-md bg-muted px-2 text-[11.5px]">on {typeLabel(p.scope)}</span>
                    </label>
                  )
                })}
              </div>
            )}
          </div>
        )
      })}
    </section>
  )
}
