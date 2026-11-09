import type { ReactNode } from 'react'
import { useSetPageCrumbs, type Crumb } from './PageTrail'

/**
 * The one header every page uses. A page never sets its own width: the layout
 * owns the content container. `crumbs` adds what comes after the section in
 * the trail, `media` sits left of the title, `meta` beside it, `action` right.
 */
export function PageHeader({
  title,
  desc,
  action,
  crumbs,
  media,
  meta,
}: {
  title: ReactNode
  desc?: ReactNode
  action?: ReactNode
  crumbs?: Crumb[]
  media?: ReactNode
  meta?: ReactNode
}) {
  useSetPageCrumbs(crumbs)
  return (
    <div className="flex items-start justify-between gap-4 max-[600px]:flex-col max-[600px]:items-stretch">
      <div className="flex min-w-0 items-center gap-3.5">
        {media && <div className="shrink-0">{media}</div>}
        <div className="min-w-0">
          <div className="flex items-center gap-2.5">
            <h1 className="truncate text-[22px] font-bold tracking-[-0.4px] max-[600px]:text-[19px]">{title}</h1>
            {meta}
          </div>
          {desc && <p className="mt-1.5 text-[13px] text-muted-foreground">{desc}</p>}
        </div>
      </div>
      {action && <div className="flex shrink-0 items-center gap-2 max-[600px]:flex-wrap">{action}</div>}
    </div>
  )
}

/** A capped column for content that reads badly wide (forms, prose). */
export function Narrow({ width = 760, className = '', children }: { width?: number; className?: string; children: ReactNode }) {
  return (
    <div className={`w-full ${className}`} style={{ maxWidth: width }}>
      {children}
    </div>
  )
}

/** A titled block inside a page: owns the gap between its header and body. */
export function Section({ title, desc, action, children }: { title: ReactNode; desc?: ReactNode; action?: ReactNode; children: ReactNode }) {
  return (
    <section className="mt-9 first:mt-0">
      <div className="mb-3 flex items-end justify-between gap-3">
        <div className="min-w-0">
          <h2 className="text-[15px] font-semibold tracking-[-0.2px]">{title}</h2>
          {desc && <p className="mt-0.5 text-[12px] text-muted-foreground">{desc}</p>}
        </div>
        {action && <div className="flex shrink-0 items-center gap-2">{action}</div>}
      </div>
      {children}
    </section>
  )
}
