import { TriangleAlert } from 'lucide-react'
import { CopyField, Field } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'

/** Shows a sign-in client's secret once. Hayat keeps only its hash. */
export function SecretDialog({ clientId, secret, onClose }: { clientId: string; secret: string; onClose: () => void }) {
  return (
    <Dialog open={!!secret} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg" onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle className="text-[15px]">Copy the client secret now</DialogTitle>
          <DialogDescription className="text-[13px]">Put it in the application's secret store. Hayat can't show it again.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <Field label="Client id">
            <CopyField value={clientId} />
          </Field>
          <Field label="Client secret">
            <CopyField value={secret} />
          </Field>
          <p className="flex items-start gap-2 rounded-[10px] border border-warning/30 bg-warning/10 px-3 py-2 text-[12px] text-warning">
            <TriangleAlert className="mt-0.5 size-3.5 shrink-0" /> Lost it? Rotate the secret from the client's page; the old one stops working at once.
          </p>
        </div>
        <DialogFooter>
          <Button onClick={onClose}>I've stored it</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
