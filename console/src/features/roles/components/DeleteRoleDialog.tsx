import { Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { CatalogService, type Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { useClient } from '@/shared/api/client'
import { ErrorState, LoadingState } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { plural } from '../lib/roles'
import { RoleGrantsList, usageLine, useRoleGrants } from './RoleGrants'

/**
 * Deleting a role. One nobody holds is confirmed and deleted; one still
 * granted can't be (the server refuses), so the dialog lists those grants and
 * links each to the tree, where it can be removed.
 */
export function DeleteRoleDialog({ role, onClose, onDeleted }: { role: Role; onClose: () => void; onDeleted: () => void }) {
  const catalog = useClient(CatalogService)
  if (role.grantCount === 0) {
    return (
      <ConfirmDialog
        open
        onOpenChange={(o) => !o && onClose()}
        title={`Delete ${role.name}?`}
        body="Nobody holds this role, so deleting it changes no one's access. This can't be undone."
        confirm="Delete role"
        destructive
        onConfirm={async () => {
          await catalog.deleteRole({ id: role.id })
          toast.success('Role deleted')
          onDeleted()
        }}
      />
    )
  }
  return <InUseDialog role={role} onClose={onClose} />
}

function InUseDialog({ role, onClose }: { role: Role; onClose: () => void }) {
  const { data, error, loading, reload } = useRoleGrants(role.id, role.grantCount)
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <div className="flex items-center gap-3">
            <span className="flex size-10 shrink-0 items-center justify-center rounded-[11px] bg-destructive/10 text-destructive">
              <Trash2 className="size-[19px]" />
            </span>
            <div className="min-w-0">
              <DialogTitle className="text-[15px]">{role.name} is still in use</DialogTitle>
              <p className="text-[12px] text-muted-foreground">
                Granted {plural(role.grantCount, 'time')}
                {data && ` · ${usageLine(data.grants, data.truncated)}`}
              </p>
            </div>
          </div>
          <DialogDescription className="pt-1 text-[13px]">
            A role can be deleted only once nobody holds it. Remove these grants first, or keep the role and change what it allows instead.
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[50vh] overflow-y-auto">
          {error ? <ErrorState message={error} onRetry={reload} /> : loading && !data ? <LoadingState /> : <RoleGrantsList grants={data?.grants ?? []} />}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Close
          </Button>
          <Button variant="destructive" disabled>
            Delete role
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
