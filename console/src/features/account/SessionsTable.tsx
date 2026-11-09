import { useState } from 'react'
import { MonitorSmartphone } from 'lucide-react'
import type { Session } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { formatDateTime, formatRelative } from '@/shared/lib/datetime'
import { EmptyState, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

/** A readable device name from a user agent, good enough to recognise a browser. */
function device(ua: string) {
  if (!ua) return 'Unknown device'
  const browser = /Edg\//.test(ua) ? 'Edge' : /Chrome\//.test(ua) ? 'Chrome' : /Firefox\//.test(ua) ? 'Firefox' : /Safari\//.test(ua) ? 'Safari' : 'Browser'
  const os = /Mac OS X/.test(ua) ? 'macOS' : /Windows/.test(ua) ? 'Windows' : /Android/.test(ua) ? 'Android' : /iPhone|iPad/.test(ua) ? 'iOS' : /Linux/.test(ua) ? 'Linux' : ''
  return os ? `${browser} on ${os}` : browser
}

export function SessionsTable({ sessions, onRevoke }: { sessions: Session[]; onRevoke: (s: Session) => Promise<void> }) {
  const [target, setTarget] = useState<Session | null>(null)
  if (sessions.length === 0) return <EmptyState icon={MonitorSmartphone} title="No signed-in sessions" />
  return (
    <>
      <div className="overflow-x-auto rounded-xl border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="text-[11px]">Device</TableHead>
              <TableHead className="text-[11px]">Signed in</TableHead>
              <TableHead className="text-[11px]">Last active</TableHead>
              <TableHead className="w-24" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {sessions.map((s) => (
              <TableRow key={s.id}>
                <TableCell>
                  <div className="flex items-center gap-2 text-[13px] font-medium">
                    {device(s.userAgent)}
                    {s.current && <Pill className="border-primary/30 text-primary">This session</Pill>}
                  </div>
                  <div className="text-[12px] text-faint">{s.ip || 'unknown address'}</div>
                </TableCell>
                <TableCell className="text-[12px] text-muted-foreground">{formatDateTime(s.authTime)}</TableCell>
                <TableCell className="text-[12px] text-muted-foreground">{formatRelative(s.lastSeenAt)}</TableCell>
                <TableCell className="text-right">
                  <Button variant="ghost" size="sm" onClick={() => setTarget(s)}>
                    Sign out
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <ConfirmDialog
        open={!!target}
        onOpenChange={(o) => !o && setTarget(null)}
        title="End this session?"
        body={
          target?.current
            ? 'This is the session you are using. You will be signed out of Hayat in this browser.'
            : 'That browser is signed out of Hayat, and the applications it signed in to lose their refresh tokens.'
        }
        confirm="Sign out"
        destructive
        onConfirm={async () => {
          if (target) await onRevoke(target)
        }}
      />
    </>
  )
}
