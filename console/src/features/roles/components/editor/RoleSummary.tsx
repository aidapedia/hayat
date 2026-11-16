import { AlertTriangle, Check } from 'lucide-react'
import type { PermissionInfo } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { Button } from '@/shared/ui/button'
import { grantableLine, groupByService, permLabel, plural, typeLabel, unreachable, type Catalog, type ServiceMeta } from '../../lib/roles'
import { ServiceMark } from '../marks'

/**
 * The role as it will be, written out in plain words while it's edited: what
 * it allows, where it can be granted, what won't work (with a one-click fix),
 * and, when editing, what changes for the people who already hold it.
 */
export function RoleSummary({
  name,
  catalog,
  services,
  perms,
  appliesTo,
  original,
  grantCount,
  saving,
  canSave,
  saveLabel,
  onAllow,
  onSave,
}: {
  name: string
  catalog: Catalog
  services: ServiceMeta[]
  perms: PermissionInfo[]
  appliesTo: string[]
  /** The permissions before this edit; absent when creating. */
  original?: string[]
  grantCount: number
  saving: boolean
  canSave: boolean
  saveLabel: string
  onAllow: (type: string) => void
  onSave: () => void
}) {
  const groups = groupByService(
    perms.map((p) => p.key),
    catalog,
    services,
  )
  const missing = [...unreachable(perms, catalog.types, appliesTo)]
  const added = original ? perms.filter((p) => !original.includes(p.key)).length : 0
  const removed = original ? original.filter((k) => !perms.some((p) => p.key === k)).length : 0

  return (
    <div className="overflow-hidden rounded-2xl border bg-background">
      <div className="flex flex-col gap-1.5 border-b px-[18px] py-4">
        <span className="text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">Preview</span>
        <span className="text-[16px] font-semibold">{name.trim() || 'Untitled role'}</span>
        <span className="text-[12.5px] text-muted-foreground">
          {plural(perms.length, 'permission')} across {plural(groups.length, 'service')}
        </span>
      </div>

      <div className="flex flex-col gap-3 border-b px-[18px] py-3.5">
        <span className="text-[12px] font-semibold text-muted-foreground">Someone with this role can</span>
        {groups.length === 0 && <span className="text-[12.5px] text-muted-foreground">Nothing yet. Tick at least one permission.</span>}
        {groups.map((g) => (
          <div key={g.service.id} className="flex flex-col gap-1.5">
            <span className="flex items-center gap-2 text-[12.5px] font-semibold">
              <ServiceMark service={g.service} size="xs" /> In {g.service.name}
            </span>
            <ul className="flex flex-col gap-1 pl-[26px]">
              {g.perms.map((p) => (
                <li key={p.key} className="flex items-start gap-1.5 text-[12.5px]">
                  <Check className="mt-0.5 size-3.5 shrink-0 text-primary" strokeWidth={2.6} />
                  {permLabel(p)}
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>

      <div className="flex flex-col gap-1 border-b px-[18px] py-3.5">
        <span className="text-[12px] font-semibold text-muted-foreground">Grantable on</span>
        <span className="text-[12.5px]">{appliesTo.length ? `${grantableLine(appliesTo)}, and everything below` : 'Any node'}</span>
      </div>

      {missing.length > 0 && (
        <div className="flex flex-col gap-3 border-b bg-warning/8 px-[18px] py-3.5">
          <span className="flex items-center gap-1.5 text-[12px] font-semibold text-warning">
            <AlertTriangle className="size-3.5" />
            {missing.length === 1 ? "1 thing won't work as expected" : `${missing.length} things won't work as expected`}
          </span>
          {missing.map(([scope, list]) => (
            <div key={scope} className="flex flex-col gap-1.5">
              <span className="text-[12.5px]">
                {list.map(permLabel).join(', ')} {list.length === 1 ? 'is' : 'are'} checked on {typeLabel(scope)}, but this role can only be granted on {grantableLine(appliesTo)}.
              </span>
              <Button type="button" variant="outline" size="sm" className="self-start" onClick={() => onAllow(scope)}>
                Also allow granting on {typeLabel(scope)}
              </Button>
            </div>
          ))}
        </div>
      )}

      {original && (added > 0 || removed > 0) && (
        <div className="flex flex-col gap-2 border-b px-[18px] py-3.5">
          <span className="text-[12px] font-semibold text-muted-foreground">Changes</span>
          <div className="flex flex-wrap gap-2">
            <span className="inline-flex h-[22px] items-center rounded-md bg-success/12 px-2 text-[11.5px] font-semibold text-success">+{added} added</span>
            <span className="inline-flex h-[22px] items-center rounded-md bg-destructive/10 px-2 text-[11.5px] font-semibold text-destructive">−{removed} removed</span>
          </div>
          {grantCount > 0 && (
            <span className="text-[12px] text-muted-foreground">
              Saving updates the {plural(grantCount, 'existing grant')} of this role straight away. People gain or lose access on their next request.
            </span>
          )}
        </div>
      )}

      <div className="px-[18px] py-3.5">
        <Button type="button" className="w-full" disabled={!canSave || saving} onClick={onSave}>
          {saving ? 'Saving…' : saveLabel}
        </Button>
      </div>
    </div>
  )
}
