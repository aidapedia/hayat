import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

/**
 * The breadcrumb trail. The layout works out root → section from the address;
 * a page adds only what comes after (a record's name) through PageHeader's
 * `crumbs`.
 */
export type Crumb = { label: string; to?: string }

const Ctx = createContext<{ crumbs: Crumb[]; setCrumbs: (c: Crumb[]) => void }>({ crumbs: [], setCrumbs: () => {} })

export function PageTrailProvider({ children }: { children: ReactNode }) {
  const [crumbs, setCrumbs] = useState<Crumb[]>([])
  return <Ctx.Provider value={{ crumbs, setCrumbs }}>{children}</Ctx.Provider>
}

export const usePageCrumbs = () => useContext(Ctx).crumbs

export function useSetPageCrumbs(crumbs: Crumb[] | undefined) {
  const { setCrumbs } = useContext(Ctx)
  const key = JSON.stringify(crumbs ?? [])
  useEffect(() => {
    setCrumbs(crumbs ?? [])
    return () => setCrumbs([])
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, setCrumbs])
}
