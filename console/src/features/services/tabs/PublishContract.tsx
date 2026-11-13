import { Link, Navigate, useNavigate } from 'react-router'
import { Clock } from 'lucide-react'
import type { GetServiceResponse } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { Button } from '@/shared/ui/button'
import { ContractEditor } from '../contract/ContractEditor'
import { ownPending } from '../lib'

/** /services/:id/contract/publish — publishing a version, inside the service's Contract tab. */
export function PublishContract({ data, onPublished }: { data: GetServiceResponse; onPublished: () => unknown }) {
  const s = data.service!
  const navigate = useNavigate()
  const back = `/services/${s.id}/contract`
  if (!data.canManage) return <Navigate to={back} replace />

  const pending = ownPending(data.contracts)
  if (pending) {
    return (
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-warning/40 bg-warning/5 px-4 py-3.5 text-[13px]">
        <span className="flex items-start gap-2.5">
          <Clock className="mt-0.5 size-4 shrink-0 text-warning" />
          Version {pending.version} waits for {pending.namespaces.join(', ')} to approve it. It must be approved or rejected before another version is published.
        </span>
        <Button variant="outline" asChild>
          <Link to={back}>Back to the contract</Link>
        </Button>
      </div>
    )
  }

  return (
    <ContractEditor
      serviceId={s.id}
      contracts={data.contracts}
      onCancel={() => navigate(back)}
      onPublished={async () => {
        await onPublished()
        navigate(back)
      }}
    />
  )
}
