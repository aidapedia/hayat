import { Lock, ShieldHalf } from 'lucide-react'
import { cn } from '@/shared/lib/utils'
import { CUSTOM_TONE, sourceOf, type ServiceMeta } from '../lib/roles'
import type { Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'

/** A service's monogram tile, coloured the same on every roles page. */
export function ServiceMark({ service, size = 'sm', className }: { service: ServiceMeta; size?: 'xs' | 'sm' | 'md' | 'lg'; className?: string }) {
  const sizes = { xs: 'size-[18px] rounded-[5px] text-[10px]', sm: 'size-5 rounded-md text-[10.5px]', md: 'size-[30px] rounded-lg text-[13px]', lg: 'size-10 rounded-[11px] text-[17px]' }
  return (
    <span aria-hidden className={cn('inline-flex shrink-0 items-center justify-center font-bold', sizes[size], service.tone, className)}>
      {service.mono}
    </span>
  )
}

/** A role's shield, tinted by who owns it. */
export function RoleIcon({ role, services, className }: { role: Role; services: ServiceMeta[]; className?: string }) {
  return (
    <span className={cn('inline-flex size-9 shrink-0 items-center justify-center rounded-[10px]', toneOf(role, services), className)}>
      <ShieldHalf className="size-[18px]" />
    </span>
  )
}

/** Built-in, From <service> or Custom: who owns the role, and so what may change it. */
export function RoleSourceBadge({ role, services }: { role: Role; services: ServiceMeta[] }) {
  const src = sourceOf(role)
  const label = src === 'builtin' ? 'Built-in' : src === 'custom' ? 'Custom' : `From ${services.find((s) => s.id === role.service)?.name ?? role.service}`
  return (
    <span className={cn('inline-flex h-5 shrink-0 items-center gap-1 rounded-md px-1.5 text-[11px] font-semibold', toneOf(role, services))}>
      {src === 'builtin' && <Lock className="size-3" />}
      {label}
    </span>
  )
}

function toneOf(role: Role, services: ServiceMeta[]) {
  const src = sourceOf(role)
  if (src === 'builtin') return 'bg-muted text-muted-foreground'
  if (src === 'custom') return CUSTOM_TONE
  return services.find((s) => s.id === role.service)?.tone ?? 'bg-muted text-muted-foreground'
}
