import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { errorMessage } from '@/shared/api/client'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/shared/ui/alert-dialog'
import { Input } from '@/shared/ui/input'

/**
 * Confirm before an action that can't be undone. `typeToConfirm` asks for a
 * word first, for the destructive ones. The dialog stays open, showing the
 * failure, if the action throws.
 */
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  body,
  confirm,
  destructive,
  typeToConfirm,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  body: ReactNode
  confirm: string
  destructive?: boolean
  typeToConfirm?: string
  onConfirm: () => Promise<void>
}) {
  const [busy, setBusy] = useState(false)
  const [typed, setTyped] = useState('')
  const blocked = !!typeToConfirm && typed !== typeToConfirm

  const run = async (e: React.MouseEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await onConfirm()
      onOpenChange(false)
      setTyped('')
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle className="text-[15px]">{title}</AlertDialogTitle>
          <AlertDialogDescription className="text-[13px]">{body}</AlertDialogDescription>
        </AlertDialogHeader>
        {typeToConfirm && (
          <div className="flex flex-col gap-1.5">
            <label htmlFor="confirm-word" className="text-[12px] text-muted-foreground">
              Type <span className="font-mono font-semibold text-foreground">{typeToConfirm}</span> to confirm
            </label>
            <Input id="confirm-word" value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" />
          </div>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={run}
            disabled={busy || blocked}
            className={destructive ? 'bg-destructive text-white hover:bg-destructive/90' : undefined}
          >
            {busy ? 'Working…' : confirm}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
