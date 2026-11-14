import { ChevronLeft, ChevronRight } from 'lucide-react'
import { cn } from '@/shared/lib/utils'
import { Button } from '@/shared/ui/button'
import { Combobox } from '@/shared/ui/combobox'

/**
 * The footer under every table, the same as tabletsgo's: "1–10 of 42", a
 * rows-per-page picker and page controls. Sits inside the table card with
 * `className="border-t"`.
 *
 * Purely controlled. Pair it with `usePagination` (a list already loaded) or
 * `useServerPagination` (a list the API pages by token). A token-paged list
 * doesn't know its total: `total` is left out, `shown` gives the rows on this
 * page, and `hasMore` draws a trailing "…" past the last page reached so far.
 */
export type PaginationProps = {
  /** 1-based. */
  page: number
  /** Pages that can be opened now. */
  pageCount: number
  pageSize: number
  onPageChange: (page: number) => void
  onPageSizeChange?: (size: number) => void
  pageSizeOptions?: number[]
  /** Rows in the whole list, when known. */
  total?: number
  /** Rows on this page — needed when `total` is unknown. */
  shown?: number
  /** More pages exist beyond `pageCount` (token paging). */
  hasMore?: boolean
  loading?: boolean
  className?: string
}

// Page numbers around the current one, with '…' gaps: 1 … 4 [5] 6 … 20.
function pageItems(page: number, pageCount: number): (number | 'gap')[] {
  if (pageCount <= 7) return Array.from({ length: pageCount }, (_, i) => i + 1)
  const around = [page - 1, page, page + 1].filter((p) => p > 1 && p < pageCount)
  const items: (number | 'gap')[] = [1]
  if (around[0] > 2) items.push('gap')
  items.push(...around)
  if (around[around.length - 1] < pageCount - 1) items.push('gap')
  items.push(pageCount)
  return items
}

export function Pagination({
  page,
  pageCount,
  pageSize,
  onPageChange,
  onPageSizeChange,
  pageSizeOptions = [10, 25, 50, 100],
  total,
  shown,
  hasMore,
  loading,
  className,
}: PaginationProps) {
  const from = total === 0 || shown === 0 ? 0 : (page - 1) * pageSize + 1
  const to = total !== undefined ? Math.min(page * pageSize, total) : from + (shown ?? pageSize) - 1
  const go = (p: number) => onPageChange(Math.min(pageCount, Math.max(1, p)))
  const items = pageItems(page, pageCount)
  if (hasMore) items.push('gap')

  return (
    <div className={cn('flex flex-wrap items-center justify-between gap-3 px-4 py-3 text-[12px] text-muted-foreground', className)}>
      <div className="flex items-center gap-2">
        <span className="tabular-nums">
          {from}–{to}
          {total !== undefined && ` of ${total}`}
        </span>
        {onPageSizeChange && (
          <>
            <span className="text-faint">·</span>
            <Combobox
              aria-label="Rows per page"
              variant="ghost"
              side="top"
              value={String(pageSize)}
              onChange={(v) => onPageSizeChange(Number(v))}
              options={pageSizeOptions.map((n) => ({ value: String(n), label: `${n} rows` }))}
              className="w-auto"
              contentClassName="min-w-32"
            />
          </>
        )}
      </div>

      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon-xs"
          className="disabled:pointer-events-none disabled:opacity-30"
          disabled={page <= 1 || loading}
          onClick={() => go(page - 1)}
          aria-label="Previous page"
        >
          <ChevronLeft />
        </Button>
        {items.map((item, i) =>
          item === 'gap' ? (
            <span key={`gap-${i}`} className="px-1 text-faint">
              …
            </span>
          ) : (
            <button
              key={item}
              type="button"
              onClick={() => go(item)}
              disabled={loading}
              aria-current={item === page ? 'page' : undefined}
              className={cn(
                'h-6 min-w-6 rounded px-1.5 text-[11px] font-semibold tabular-nums transition-colors',
                item === page ? 'bg-muted text-foreground' : 'text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              {item}
            </button>
          ),
        )}
        <Button
          variant="ghost"
          size="icon-xs"
          className="disabled:pointer-events-none disabled:opacity-30"
          disabled={page >= pageCount || loading}
          onClick={() => go(page + 1)}
          aria-label="Next page"
        >
          <ChevronRight />
        </Button>
      </div>
    </div>
  )
}
