import { useState } from 'react'
import { RotateCw } from 'lucide-react'
import { toast } from 'sonner'
import { KeyService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDateTime } from '@/shared/lib/datetime'
import { ErrorState, LoadingState, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { PageHeader, Section } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

const STAGES = [
  { status: 'next', text: 'Published before it signs anything, so every application already has it cached when it takes over.' },
  { status: 'active', text: 'Signs every new ID and access token.' },
  { status: 'retired', text: 'Still published for 48 hours, so tokens it signed keep verifying until they expire.' },
]

export default function KeysPage() {
  const keys = useClient(KeyService)
  const { data, error, loading, reload, setData } = useQuery(() => keys.listSigningKeys({}).then((r) => r.keys), [keys])
  const [confirming, setConfirming] = useState(false)

  return (
    <>
      <PageHeader
        title="Signing keys"
        desc="The ES256 keys that sign Hayat's tokens. They rotate on their own; rotate now if a key may have leaked."
        action={
          <Button size="lg" variant="outline" onClick={() => setConfirming(true)}>
            <RotateCw /> Rotate now
          </Button>
        }
      />
      <div className="mt-7">
        <div className="grid grid-cols-3 gap-3 max-[900px]:grid-cols-1">
          {STAGES.map((s) => (
            <div key={s.status} className="rounded-xl border p-4">
              <StatusBadge status={s.status} />
              <p className="mt-2 text-[12px] text-muted-foreground">{s.text}</p>
            </div>
          ))}
        </div>
        <Section title="Keys">
          {error ? (
            <ErrorState message={error} onRetry={reload} />
          ) : loading && !data ? (
            <LoadingState />
          ) : (
            <div className="overflow-x-auto rounded-xl border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-[11px]">Key id</TableHead>
                    <TableHead className="text-[11px]">Stage</TableHead>
                    <TableHead className="text-[11px]">Created</TableHead>
                    <TableHead className="text-[11px]">Signing since</TableHead>
                    <TableHead className="text-[11px]">Retired</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(data ?? []).map((k) => (
                    <TableRow key={k.id}>
                      <TableCell className="font-mono text-[12px]">{k.id}</TableCell>
                      <TableCell>
                        <StatusBadge status={k.status} />
                      </TableCell>
                      <TableCell className="text-[12px] text-muted-foreground">{formatDateTime(k.createdAt)}</TableCell>
                      <TableCell className="text-[12px] text-muted-foreground">{formatDateTime(k.activatedAt)}</TableCell>
                      <TableCell className="text-[12px] text-muted-foreground">{formatDateTime(k.retiredAt)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </Section>
      </div>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title="Rotate the signing keys?"
        body="The next key starts signing now and a new next key is made. Tokens signed by the current key stay valid until they expire. Pods pick up the change within 30 seconds."
        confirm="Rotate keys"
        onConfirm={async () => {
          const res = await keys.rotateSigningKeys({})
          setData(res.keys)
          toast.success('Signing keys rotated')
        }}
      />
    </>
  )
}
