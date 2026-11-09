import { useCallback, useEffect, useRef, useState, type DependencyList } from 'react'
import { errorMessage } from '@/shared/api/client'

/**
 * Load something when the page opens and whenever deps change. Small on
 * purpose: the console reads a page of data and reloads it after a change.
 */
export function useQuery<T>(fn: () => Promise<T>, deps: DependencyList) {
  const [data, setData] = useState<T | undefined>(undefined)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const seq = useRef(0)

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const run = useCallback(fn, deps)

  const reload = useCallback(async () => {
    const mine = ++seq.current
    setLoading(true)
    try {
      const value = await run()
      if (mine === seq.current) {
        setData(value)
        setError(null)
      }
    } catch (err) {
      if (mine === seq.current) setError(errorMessage(err))
    } finally {
      if (mine === seq.current) setLoading(false)
    }
  }, [run])

  useEffect(() => {
    void reload()
  }, [reload])

  return { data, error, loading, reload, setData }
}
