import { ScrollText } from 'lucide-react'
import { AuditService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { useClient } from '@/shared/api/client'
import { useServerPagination } from '@/shared/ui/table/usePagination'
import { formatDateTime } from '@/shared/lib/datetime'
import { EmptyState, ErrorState, LoadingState } from '@/shared/ui/bits'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Pagination } from '@/shared/ui/table/Pagination'
import { actionLabel, actorLabel } from './labels'

export default function AuditPage() {
  const audit = useClient(AuditService)
  const list = useServerPagination(
    (token, pageSize) => audit.listAuditEvents({ pageSize, pageToken: token }).then((r) => ({ items: r.events, next: r.nextPageToken })),
    [audit],
  )

  return (
    <>
      <PageHeader title="Audit log" desc="Every change made in Hayat, newest first. Rows are written in the same transaction as the change." />
      <div className="mt-7 flex flex-col gap-4">
        {list.error ? (
          <ErrorState message={list.error} onRetry={list.reload} />
        ) : list.loading && list.items.length === 0 ? (
          <LoadingState />
        ) : list.items.length === 0 ? (
          <EmptyState icon={ScrollText} title="Nothing recorded yet" />
        ) : (
          <div className="overflow-hidden rounded-xl border">
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
                {list.items.map((e) => (
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
            <Pagination {...list.pager} className="border-t" />
          </div>
        )}
      </div>
    </>
  )
}
