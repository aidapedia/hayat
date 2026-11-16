import { useCallback, useEffect, useMemo, useRef, useState, type DependencyList } from 'react'
import { errorMessage } from '@/shared/api/client'
import type { PaginationProps } from './Pagination'

/** Rows per page a table starts with. */
export const PAGE_SIZE = 25

/**
 * Client-side paging for a list already loaded — tabletsgo's `usePagination`.
 *
 *   const { pageRows, pager } = usePagination(rows)
 *   …render pageRows… <Pagination {...pager} className="border-t" />
 *
 * `resetKey` is any value that means "a different list": when it changes the
 * pager goes back to page 1. The page is clamped when the list shrinks.
 */
export function usePagination<T>(rows: readonly T[], { pageSize: initial = PAGE_SIZE, resetKey }: { pageSize?: number; resetKey?: unknown } = {}) {
  const [pageSize, setPageSize] = useState(initial)
  const [page, setPage] = useState(1)
  const [lastReset, setLastReset] = useState(resetKey)

  // A different list ⇒ page 1. Done during render so the stale page never paints.
  if (resetKey !== lastReset) {
    setLastReset(resetKey)
    setPage(1)
  }

  const total = rows.length
  const pageCount = Math.max(1, Math.ceil(total / pageSize))
  const current = Math.min(page, pageCount)
  const offset = (current - 1) * pageSize
  const pageRows = useMemo(() => rows.slice(offset, offset + pageSize), [rows, offset, pageSize])

  const pager: PaginationProps = {
    page: current,
    pageCount,
    total,
    pageSize,
    onPageChange: setPage,
    onPageSizeChange: (n) => {
      setPageSize(n)
      setPage(1)
    },
  }
  return { pageRows, offset, pager }
}

/**
 * A list the API pages by opaque token (`page_token` → `next_page_token`),
 * read one page at a time. The API has no total and no "jump to page n", so
 * the pager offers every page reached so far plus the next one, and a
 * trailing "…" until the last page has been seen. Restarts on page 1 when
 * deps change (a new search) or the page size does.
 */
export function useServerPagination<T>(
  fetchPage: (token: string, pageSize: number) => Promise<{ items: T[]; next: string }>,
  deps: DependencyList,
  { pageSize: initial = PAGE_SIZE }: { pageSize?: number } = {},
) {
  const [pageSize, setPageSize] = useState(initial)
  const [items, setItems] = useState<T[]>([])
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  // tokens[i] reads page i + 1; `ended` once a page came back without a next token.
  const tokens = useRef<string[]>([''])
  const ended = useRef(false)
  const seq = useRef(0)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const fetcher = useCallback(fetchPage, deps)

  const load = useCallback(
    async (target: number): Promise<void> => {
      const mine = ++seq.current
      setLoading(true)
      try {
        const res = await fetcher(tokens.current[target - 1], pageSize)
        if (mine !== seq.current) return
        // A page emptied by a delete: step back rather than show nothing.
        if (res.items.length === 0 && target > 1) return load(target - 1)
        if (!res.next) {
          tokens.current = tokens.current.slice(0, target)
          ended.current = true
        } else if (tokens.current[target] !== res.next) {
          // What follows changed (rows added or removed): forget the pages past this one.
          tokens.current = [...tokens.current.slice(0, target), res.next]
          ended.current = false
        }
        setPage(target)
        setItems(res.items)
        setError(null)
      } catch (err) {
        if (mine === seq.current) setError(errorMessage(err))
      } finally {
        if (mine === seq.current) setLoading(false)
      }
    },
    [fetcher, pageSize],
  )

  useEffect(() => {
    tokens.current = ['']
    ended.current = false
    void load(1)
  }, [load])

  const pager: PaginationProps = {
    page,
    pageCount: tokens.current.length,
    pageSize,
    shown: items.length,
    hasMore: !ended.current,
    loading,
    onPageChange: (p) => void load(p),
    onPageSizeChange: setPageSize,
  }
  return { items, loading, error, pager, reload: () => load(page) }
}
