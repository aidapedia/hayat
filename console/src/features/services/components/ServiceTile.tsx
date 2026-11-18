import { LogoTile } from '@/shared/ui/bits'

/** A service's logo, or its initial in a tile; Hayat's own is drawn inverted. */
export function ServiceTile({ name, src, builtin, size = 34, className }: { name: string; src?: string; builtin?: boolean; size?: number; className?: string }) {
  return <LogoTile name={name} src={src} size={size} inverted={builtin} className={className} />
}
