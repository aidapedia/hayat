import { useMemo, useState } from 'react'
import { ChevronLeft, ChevronRight, Lock, UserMinus } from 'lucide-react'
import type { Grant, Node } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { cn } from '@/shared/lib/utils'
import { PersonRow, SearchInput } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Checkbox } from '@/shared/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { directGrant, wayText, whyNotRemovable, type Block } from './access'

const PAGE = 8

/**
 * Everyone one role reaches at a node, in a modal: searchable, a page at a
 * time. Only a grant made on this node to the person themselves can be
 * ungranted here; everyone else shows a lock and where to change it instead.
 */
export function AccessDialog({
  block,
  node,
  tenantName,
  onRemove,
  onClose,
}: {
  block: Block
  node: Node
  tenantName: string
  /** Asks to remove these grants; absent when the viewer may not grant here. */
  onRemove?: (grants: Grant[]) => void
  onClose: () => void
}) {
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(0)
  const [picked, setPicked] = useState<Set<string>>(new Set())

  const removable = (id: string) => (onRemove ? directGrant(block, id) : undefined)
  const people = useMemo(() => {
    const q = query.trim().toLowerCase()
    return q ? block.people.filter((p) => `${p.user.name} ${p.user.email}`.toLowerCase().includes(q)) : block.people
  }, [block.people, query])
  const pages = Math.max(1, Math.ceil(people.length / PAGE))
  const current = Math.min(page, pages - 1)
  const shown = people.slice(current * PAGE, current * PAGE + PAGE)

  // Only grants still there after a reload count as picked.
  const pickedGrants = block.people.flatMap((p) => (picked.has(p.user.id) ? (removable(p.user.id) ?? []) : []))
  const pageRemovable = shown.filter((p) => removable(p.user.id))
  const allOnPage = pageRemovable.length > 0 && pageRemovable.every((p) => picked.has(p.user.id))
  const setMany = (ids: string[], on: boolean) =>
    setPicked((cur) => {
      const next = new Set(cur)
      for (const id of ids) {
        if (on) next.add(id)
        else next.delete(id)
      }
      return next
    })

  const count = block.people.length
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="gap-0 p-0 sm:max-w-xl">
        <DialogHeader className="border-b p-4 pr-12">
          <DialogTitle className="text-[15px]">{block.title}</DialogTitle>
          <DialogDescription className="text-[12px]">
            {count} {count === 1 ? 'person reaches' : 'people reach'} {node.name} this way
            {block.note && (
              <>
                {' · '}
                <span className={cn(block.mono && 'font-mono')}>{block.note}</span>
              </>
            )}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-wrap items-center gap-3 border-b px-4 py-3">
          {onRemove && (
            <Checkbox
              aria-label="Select everyone on this page who can be ungranted"
              checked={allOnPage}
              disabled={pageRemovable.length === 0}
              onCheckedChange={(v) => setMany(pageRemovable.map((p) => p.user.id), v === true)}
            />
          )}
          <SearchInput
            className="flex-1"
            placeholder="Search people"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setPage(0)
            }}
          />
        </div>

        <ul className={cn('divide-y overflow-y-auto', pages > 1 ? 'h-[384px]' : 'min-h-24')}>
          {shown.length === 0 && <li className="px-4 py-10 text-center text-[12px] text-faint">Nobody here.</li>}
          {shown.map((p) => {
            const can = !!removable(p.user.id)
            const on = picked.has(p.user.id)
            return (
              <li key={p.user.id} className={cn('flex h-12 items-center gap-3 px-4', on && 'bg-accent/40')}>
                {onRemove &&
                  (can ? (
                    <Checkbox
                      aria-label={`Select ${p.user.name || p.user.email}`}
                      checked={on}
                      onCheckedChange={(v) => setMany([p.user.id], v === true)}
                    />
                  ) : (
                    <Lock aria-label="Can't be ungranted here" className="size-4 shrink-0 text-faint" />
                  ))}
                <div className="min-w-0 flex-1">
                  <PersonRow name={p.user.name} email={p.user.email} />
                </div>
                <span className="max-w-[45%] shrink-0 text-right text-[11px]">
                  {p.ways.map((w, i) => (
                    <span key={i} className="block truncate text-muted-foreground">
                      {wayText(w, node, tenantName)}
                    </span>
                  ))}
                  {onRemove && !can && <span className="block truncate text-faint">{whyNotRemovable(p, node)}</span>}
                </span>
              </li>
            )
          })}
        </ul>

        <DialogFooter className="m-0 flex-row items-center gap-2 sm:justify-start">
          {pickedGrants.length > 0 && (
            <>
              <Button variant="destructive" size="sm" onClick={() => onRemove?.(pickedGrants)}>
                <UserMinus /> Ungrant {pickedGrants.length}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setPicked(new Set())}>
                Clear
              </Button>
            </>
          )}
          <span className="ml-auto flex items-center gap-1">
            <span className="text-[11px] whitespace-nowrap text-muted-foreground tabular-nums">
              {people.length === 0 ? '0' : `${current * PAGE + 1}–${Math.min(people.length, (current + 1) * PAGE)}`} of {people.length}
            </span>
            <Button variant="ghost" size="icon-sm" aria-label="Previous page" disabled={current === 0} onClick={() => setPage(current - 1)}>
              <ChevronLeft />
            </Button>
            <Button variant="ghost" size="icon-sm" aria-label="Next page" disabled={current >= pages - 1} onClick={() => setPage(current + 1)}>
              <ChevronRight />
            </Button>
          </span>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
