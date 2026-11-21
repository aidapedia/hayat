import { useState } from 'react'
import { toast } from 'sonner'
import { Ban } from 'lucide-react'
import type { Enrollment } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { RegistryService } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { formatDateTime } from '@/shared/lib/datetime'
import { PersonRow, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Pagination } from '@/shared/ui/table/Pagination'
import { usePagination } from '@/shared/ui/table/usePagination'

const STATUS: Record<string, string> = {
  open: 'border-primary/30 text-primary',
  used: 'border-success/30 text-success',
  expired: '',
  revoked: '',
}

function outcome(e: Enrollment) {
  if (e.status === 'used') return `${e.registered ? 'Registered' : 'Rotated the secret'} · ${formatDateTime(e.usedAt)}`
  if (e.status === 'revoked') return `Revoked · ${formatDateTime(e.revokedAt)}`
  if (e.status === 'expired') return `Expired · ${formatDateTime(e.expiresAt)}`
  return `Until ${formatDateTime(e.expiresAt)}`
}

/** Enrollment codes, newest first. The code itself is never shown again; an open one can be revoked. */
export function EnrollmentTable({ rows, showService, canManage, onChanged }: { rows: Enrollment[]; showService?: boolean; canManage: boolean; onChanged: () => unknown }) {
  const registry = useClient(RegistryService)
  const { pageRows, pager } = usePagination(rows)
  const [revoking, setRevoking] = useState<Enrollment | null>(null)
  const cols = showService ? 5 : 4

  return (
    <>
      <div className="overflow-hidden rounded-xl border">
        <Table>
          <TableHeader>
            <TableRow>
              {showService && <TableHead className="text-[11px]">Service</TableHead>}
              <TableHead className="text-[11px]">Made by</TableHead>
              <TableHead className="text-[11px]">Status</TableHead>
              <TableHead className="text-[11px]">When</TableHead>
              <TableHead className="w-12 text-[11px]" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {pageRows.length === 0 && (
              <TableRow>
                <TableCell colSpan={cols} className="py-8 text-center text-[12px] text-faint">
                  No enrollment codes yet.
                </TableCell>
              </TableRow>
            )}
            {pageRows.map((e) => (
              <TableRow key={e.id}>
                {showService && (
                  <TableCell>
                    <div className="text-[13px] font-semibold">{e.displayName || e.serviceId}</div>
                    <div className="font-mono text-[12px] text-faint">{e.serviceId}</div>
                  </TableCell>
                )}
                <TableCell>{e.createdBy ? <PersonRow name={e.createdBy.name} email={e.createdBy.email} src={e.createdBy.avatarUrl} /> : '—'}</TableCell>
                <TableCell>
                  <Pill className={STATUS[e.status]}>{e.status}</Pill>
                </TableCell>
                <TableCell className="text-[12px] text-muted-foreground">{outcome(e)}</TableCell>
                <TableCell className="text-right">
                  {canManage && e.status === 'open' && (
                    <Button variant="ghost" size="icon-sm" aria-label="Revoke this code" title="Revoke" onClick={() => setRevoking(e)}>
                      <Ban />
                    </Button>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <Pagination {...pager} className="border-t" />
      </div>
      <ConfirmDialog
        open={!!revoking}
        onOpenChange={(o) => !o && setRevoking(null)}
        title="Revoke this enrollment code?"
        body="Nobody can redeem it any more."
        confirm="Revoke"
        destructive
        onConfirm={async () => {
          try {
            await registry.revokeEnrollment({ id: revoking!.id })
            toast.success('Code revoked')
            await onChanged()
          } catch (err) {
            toast.error(errorMessage(err))
          }
          setRevoking(null)
        }}
      />
    </>
  )
}
