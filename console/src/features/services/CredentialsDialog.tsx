import { TriangleAlert } from 'lucide-react'
import { useAuth } from '@/features/auth'
import { CopyField, Field } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'

/** A service's API credentials, shown once. */
export function CredentialsDialog({ clientId, secret, onClose }: { clientId: string; secret: string; onClose: () => void }) {
  const { config } = useAuth()
  const env = `HAYAT_ISSUER=${config.issuer}\nHAYAT_CLIENT_ID=${clientId}\nHAYAT_CLIENT_SECRET=${secret}`
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg" onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle className="text-[15px]">Copy the service's API credentials</DialogTitle>
          <DialogDescription className="text-[13px]">
            The service calls Hayat's API with these. Hayat keeps only a hash of the secret and can't show it again.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <Field label="Client id">
            <CopyField value={clientId} />
          </Field>
          <Field label="Client secret">
            <CopyField value={secret} />
          </Field>
          <Field label="As environment variables">
            <pre className="overflow-x-auto rounded-[10px] border bg-muted/50 p-3 font-mono text-[11px] select-all">{env}</pre>
          </Field>
          <p className="flex items-start gap-2 rounded-[10px] border border-warning/30 bg-warning/10 px-3 py-2 text-[12px] text-warning">
            <TriangleAlert className="mt-0.5 size-3.5 shrink-0" /> Lost it? Rotate the secret on the service's page; the old one stops working at once.
          </p>
        </div>
        <DialogFooter>
          <Button onClick={onClose}>I've stored them</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
