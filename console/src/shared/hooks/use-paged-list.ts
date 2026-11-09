import { useCallback, useEffect, useRef, useState, type DependencyList } from 'react'
import { errorMessage } from '@/shared/api/client'

/** A list loaded a page at a time with "load more", restarted when deps change. */
export function usePagedList<T>(fetchPage: (token: string) => Promise<{ items: T[]; next: string }>, deps: DependencyList) {
  const [items, setItems] = useState<T[]>([])
  const [next, setNext] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const seq = useRef(0)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const fetcher = useCallback(fetchPage, deps)

  const load = useCallback(
    async (token: string) => {
      const mine = ++seq.current
      setLoading(true)
      try {
        const page = await fetcher(token)
        if (mine !== seq.current) return
        setItems((cur) => (token ? [...cur, ...page.items] : page.items))
        setNext(page.next)
        setError(null)
      } catch (err) {
        if (mine === seq.current) setError(errorMessage(err))
      } finally {
        if (mine === seq.current) setLoading(false)
      }
    },
    [fetcher],
  )

  useEffect(() => {
    void load('')
  }, [load])

  return { items, loading, error, hasMore: !!next, loadMore: () => load(next), reload: () => load('') }
}
