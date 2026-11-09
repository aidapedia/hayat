import { useState } from 'react'
import { ScrollText } from 'lucide-react'
import { AuditService, type AuditEvent } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDateTime } from '@/shared/lib/datetime'
import { EmptyState, ErrorState, LoadingState } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { actionLabel, actorLabel } from './labels'

export default function AuditPage() {
  const audit = useClient(AuditService)
  const [more, setMore] = useState<AuditEvent[]>([])
  const [next, setNext] = useState('')
  const [loadingMore, setLoadingMore] = useState(false)
  const [moreError, setMoreError] = useState<string | null>(null)

  const first = useQuery(async () => {
    const res = await audit.listAuditEvents({ pageSize: 50 })
    setMore([])
    setNext(res.nextPageToken)
    return res.events
  }, [audit])

  const loadMore = async () => {
    setLoadingMore(true)
    try {
      const res = await audit.listAuditEvents({ pageSize: 50, pageToken: next })
      setMore((m) => [...m, ...res.events])
      setNext(res.nextPageToken)
    } catch (err) {
      setMoreError(errorMessage(err))
    } finally {
      setLoadingMore(false)
    }
  }

  const events = [...(first.data ?? []), ...more]

  return (
    <>
      <PageHeader title="Audit log" desc="Every change made in Hayat, newest first. Rows are written in the same transaction as the change." />
      <div className="mt-7">
        {first.error ? (
          <ErrorState message={first.error} onRetry={first.reload} />
        ) : first.loading && !first.data ? (
          <LoadingState />
        ) : events.length === 0 ? (
          <EmptyState icon={ScrollText} title="Nothing recorded yet" />
        ) : (
          <>
            <div className="overflow-x-auto rounded-xl border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-48 text-[11px]">When</TableHead>
                    <TableHead className="text-[11px]">Who</TableHead>
                    <TableHead className="text-[11px]">What</TableHead>
                    <TableHead className="text-[11px]">Target</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {events.map((e) => (
                    <TableRow key={e.id}>
                      <TableCell className="text-[12px] whitespace-nowrap text-muted-foreground">{formatDateTime(e.at)}</TableCell>
                      <TableCell className="text-[13px] font-medium">{actorLabel(e.actor, e.actorName)}</TableCell>
                      <TableCell className="text-[13px]">
                        {actionLabel(e.action)}
                        {e.detailJson !== '{}' && <div className="max-w-md truncate font-mono text-[11px] text-faint">{e.detailJson}</div>}
                      </TableCell>
                      <TableCell className="max-w-56 truncate font-mono text-[11px] text-muted-foreground">{e.target}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            {moreError && <p className="mt-3 text-[12px] text-destructive">{moreError}</p>}
            {next && (
              <div className="mt-4 flex justify-center">
                <Button variant="outline" onClick={loadMore} disabled={loadingMore}>
                  {loadingMore ? 'Loading…' : 'Load older events'}
                </Button>
              </div>
            )}
          </>
        )}
      </div>
    </>
  )
}
