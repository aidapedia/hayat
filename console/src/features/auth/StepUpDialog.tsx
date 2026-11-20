import { ShieldCheck } from 'lucide-react'
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

/** Hayat's "confirm it's you" page, returning to where the person is now. */
export function reauthURL(issuer: string) {
  return `${issuer}/reauth?return_to=${encodeURIComponent(window.location.href)}`
}

/**
 * Shown when the admin API refuses a sensitive change because the person
 * hasn't confirmed it's them in the last few minutes. Confirming happens on
 * Hayat's own page (password, code or passkey), which brings them back here
 * to try again.
 */
export function StepUpDialog({ open, onOpenChange, onConfirm }: { open: boolean; onOpenChange: (open: boolean) => void; onConfirm: () => void }) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle className="flex items-center gap-2 text-[15px]">
            <ShieldCheck className="size-4 text-primary" /> Confirm it&apos;s you
          </AlertDialogTitle>
          <AlertDialogDescription className="text-[13px]">
            This change is sensitive, so Hayat asks you to confirm it&apos;s you first. You&apos;ll come straight back to this page; then try
            again.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Not now</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm}>Confirm it&apos;s me</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
