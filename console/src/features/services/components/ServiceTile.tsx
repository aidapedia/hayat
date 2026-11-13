import { cn } from '@/shared/lib/utils'

/** A service's initial in a tile; Hayat's own is drawn inverted. */
export function ServiceTile({ name, builtin, size = 34, className }: { name: string; builtin?: boolean; size?: number; className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'inline-flex shrink-0 items-center justify-center rounded-[10px] font-bold',
        builtin ? 'bg-foreground text-background' : 'bg-primary/10 text-primary',
        className,
      )}
      style={{ width: size, height: size, fontSize: Math.round(size * 0.42) }}
    >
      {(name.trim()[0] ?? '?').toUpperCase()}
    </span>
  )
}
