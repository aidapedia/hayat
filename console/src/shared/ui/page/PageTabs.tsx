import { NavLink } from 'react-router'
import { cn } from '@/shared/lib/utils'

/**
 * The tab bar under a PageHeader. A tab is an address, so a refresh keeps you
 * on it and a link lands on it.
 */
export type PageTab = { to: string; label: string; end?: boolean }

export function PageTabs({ tabs }: { tabs: PageTab[] }) {
  return (
    <div className="mt-6 flex items-center gap-5 overflow-x-auto border-b">
      {tabs.map((t) => (
        <NavLink
          key={t.to}
          to={t.to}
          end={t.end}
          className={({ isActive }) =>
            cn(
              '-mb-px shrink-0 border-b-2 pb-2.5 text-[13px] transition-colors',
              isActive ? 'border-primary font-semibold text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
            )
          }
        >
          {t.label}
        </NavLink>
      ))}
    </div>
  )
}
