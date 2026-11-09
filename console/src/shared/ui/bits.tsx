import { useState, type ComponentProps, type ReactNode } from 'react'
import Avatar from 'boring-avatars'
import { Check, Copy, Loader2, Search } from 'lucide-react'
import { toast } from 'sonner'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'

const AVATAR_COLORS = ['#1d6a54', '#3fae86', '#8fd3b6', '#d6a73a', '#2b3a35']

/** A person's generated face. Always seeded name || email, so a person looks the same in every list. */
export function PersonAvatar({ name, email, size = 32, className }: { name?: string; email?: string; size?: number; className?: string }) {
  return (
    <span className={cn('inline-flex shrink-0 overflow-hidden rounded-full', className)} style={{ width: size, height: size }}>
      <Avatar size={size} name={name || email || '?'} variant="beam" colors={AVATAR_COLORS} />
    </span>
  )
}

/** Avatar + name + email, the way a person appears in every list. */
export function PersonRow({ name, email, trailing }: { name?: string; email: string; trailing?: ReactNode }) {
  return (
    <div className="flex min-w-0 items-center gap-2.5">
      <PersonAvatar name={name} email={email} size={30} />
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="truncate text-[13px] font-semibold">{name || email}</span>
          {trailing}
        </div>
        {name && <div className="truncate text-[12px] text-muted-foreground">{email}</div>}
      </div>
    </div>
  )
}

const STATUS: Record<string, { label: string; className: string }> = {
  active: { label: 'Active', className: 'border-success/30 bg-success/10 text-success' },
  pending: { label: 'Pending', className: 'border-warning/30 bg-warning/10 text-warning' },
  disabled: { label: 'Disabled', className: 'border-destructive/30 bg-destructive/10 text-destructive' },
  next: { label: 'Next', className: 'border-border bg-muted text-muted-foreground' },
  retired: { label: 'Retired', className: 'border-border bg-muted text-faint' },
  superseded: { label: 'Superseded', className: 'border-border bg-muted text-faint' },
  rejected: { label: 'Rejected', className: 'border-destructive/30 bg-destructive/10 text-destructive' },
}

/** State as a pill, so what needs attention stands out in a list. */
export function StatusBadge({ status }: { status: string }) {
  const s = STATUS[status] ?? { label: status, className: '' }
  return (
    <Badge variant="outline" className={cn('h-5 px-1.5 text-[10px] font-bold tracking-wide uppercase', s.className)}>
      {s.label}
    </Badge>
  )
}

export function Pill({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <Badge variant="outline" className={cn('h-5 px-1.5 text-[10px] font-bold tracking-wide uppercase text-muted-foreground', className)}>
      {children}
    </Badge>
  )
}

export function LoadingState({ label = 'Loading…' }: { label?: string }) {
  return (
    <div className="flex items-center gap-2 py-10 text-[13px] text-faint">
      <Loader2 className="size-4 animate-spin" /> {label}
    </div>
  )
}

export function EmptyState({ icon: Icon, title, children }: { icon?: React.ComponentType<{ className?: string }>; title: string; children?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-dashed px-6 py-12 text-center">
      {Icon && <Icon className="mb-3 size-6 text-faint" />}
      <div className="text-[13px] font-semibold">{title}</div>
      {children && <div className="mt-1 max-w-sm text-[12px] text-muted-foreground">{children}</div>}
    </div>
  )
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-[13px] text-destructive">
      <span>{message}</span>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          Try again
        </Button>
      )}
    </div>
  )
}

export function SearchInput({ className, ...props }: ComponentProps<typeof Input>) {
  return (
    <div className={cn('relative', className)}>
      <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-faint" />
      <Input {...props} className="h-9 pl-8 text-[13px]" />
    </div>
  )
}

/** A label + control + optional hint, the field shape every form uses. */
export function Field({ label, htmlFor, hint, children }: { label: string; htmlFor?: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={htmlFor} className="text-[12px] font-semibold text-muted-foreground">
        {label}
      </Label>
      {children}
      {hint && <p className="text-[12px] text-faint">{hint}</p>}
    </div>
  )
}

/** A read-only value with a copy button: ids, URLs, a secret shown once. */
export function CopyField({ value, mono = true }: { value: string; mono?: boolean }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error('Copying is blocked here; select the text instead.')
    }
  }
  return (
    <div className="flex min-w-0 items-center gap-2 rounded-[10px] border bg-muted/50 py-1 pr-1 pl-3">
      <span className={cn('min-w-0 flex-1 truncate text-[12px] select-all', mono && 'font-mono')}>{value}</span>
      <Button type="button" variant="ghost" size="icon-sm" onClick={copy} aria-label="Copy">
        {copied ? <Check className="text-success" /> : <Copy />}
      </Button>
    </div>
  )
}

/**
 * A label/value list for a record's details. `stacked` puts each label above
 * its value, for the narrow side panels of a detail page.
 */
export function DetailList({ items, stacked }: { items: [string, ReactNode][]; stacked?: boolean }) {
  if (stacked) {
    return (
      <dl className="flex flex-col gap-3 text-[13px]">
        {items.map(([k, v]) => (
          <div key={k} className="flex min-w-0 flex-col gap-1">
            <dt className="text-[12px] text-muted-foreground">{k}</dt>
            <dd className="min-w-0 break-words">{v}</dd>
          </div>
        ))}
      </dl>
    )
  }
  return (
    <dl className="grid grid-cols-[minmax(120px,180px)_minmax(0,1fr)] gap-x-6 gap-y-3 text-[13px] max-[600px]:grid-cols-1 max-[600px]:gap-y-1">
      {items.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-muted-foreground max-[600px]:mt-2">{k}</dt>
          <dd className="min-w-0 break-words">{v}</dd>
        </div>
      ))}
    </dl>
  )
}
