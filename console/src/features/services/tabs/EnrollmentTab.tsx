import { useState } from 'react'
import { KeyRound } from 'lucide-react'
import { RegistryService, type ServiceInfo } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { ErrorState, LoadingState } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { EnrollmentDialog } from '../components/EnrollmentDialog'
import { EnrollmentTable } from '../components/EnrollmentTable'

/**
 * A service's enrollment codes. A new one lets its deployment re-enroll from
 * its setup page: a new API secret, without anyone copying it. Shown to the
 * service's owners.
 */
export function EnrollmentTab({ service: s }: { service: ServiceInfo }) {
  const registry = useClient(RegistryService)
  const { data, error, loading, reload } = useQuery(() => registry.listEnrollments({ serviceId: s.id }).then((r) => r.enrollments), [registry, s.id])
  const [making, setMaking] = useState(false)

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-2xl text-[13px] text-muted-foreground">
          A code the service pastes into its setup page to get a new API secret. One works at a time: making another revokes the last. Redeeming it
          retires the old secret at once.
        </p>
        <Button size="lg" onClick={() => setMaking(true)}>
          <KeyRound /> New enrollment code
        </Button>
      </div>
      {error ? <ErrorState message={error} onRetry={reload} /> : loading && !data ? <LoadingState /> : <EnrollmentTable rows={data ?? []} canManage onChanged={reload} />}
      {making && <EnrollmentDialog service={s} onClose={() => setMaking(false)} onMade={reload} />}
    </div>
  )
}
