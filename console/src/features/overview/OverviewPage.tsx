import type { ComponentType } from 'react'
import { Link } from 'react-router'
import { AppWindow, Building2, MonitorSmartphone, Users } from 'lucide-react'
import { OverviewService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatRelative } from '@/shared/lib/datetime'
import { CopyField, DetailList, ErrorState, LoadingState } from '@/shared/ui/bits'
import { PageHeader, Section } from '@/shared/ui/page/PageHeader'
import { actionLabel, actorLabel } from '@/features/audit/labels'

function Stat({ label, value, sub, Icon, to }: { label: string; value: number; sub?: string; Icon: ComponentType<{ className?: string }>; to?: string }) {
  const body = (
    <div className="flex h-full flex-col gap-3 rounded-xl border bg-card p-4 transition-colors hover:bg-accent/50">
      <div className="flex items-center justify-between text-[12px] text-muted-foreground">
        {label}
        <Icon className="size-4 text-faint" />
      </div>
      <div className="text-[26px] leading-none font-bold tracking-[-0.5px] tabular-nums">{value.toLocaleString()}</div>
      {sub && <div className="text-[12px] text-faint">{sub}</div>}
    </div>
  )
  return to ? <Link to={to}>{body}</Link> : body
}

export default function OverviewPage() {
  const overview = useClient(OverviewService)
  const { data, error, loading, reload } = useQuery(() => overview.getOverview({}), [overview])

  return (
    <>
      <PageHeader title="Overview" desc="Who can sign in, which applications use Hayat, and what changed recently." />
      <div className="mt-7">
        {error ? (
          <ErrorState message={error} onRetry={reload} />
        ) : loading && !data ? (
          <LoadingState />
        ) : data ? (
          <>
            <div className="grid grid-cols-4 gap-3 max-[1000px]:grid-cols-2 max-[480px]:grid-cols-1">
              <Stat label="Users" value={data.users} sub={`${data.activeUsers.toLocaleString()} active`} Icon={Users} to="/users" />
              <Stat label="Tenants" value={data.tenants} Icon={Building2} to="/tenants" />
              <Stat label="OAuth clients" value={data.clients} Icon={AppWindow} to="/services" />
              <Stat label="Signed-in sessions" value={data.activeSessions} Icon={MonitorSmartphone} />
            </div>

            <div className="mt-9 grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-x-10 gap-y-9 max-[900px]:grid-cols-1">
              <div className="min-w-0">
              <Section title="Recent activity" action={<Link to="/audit" className="text-[12px] font-semibold text-primary">Audit log</Link>}>
                {data.recent.length === 0 ? (
                  <p className="text-[13px] text-faint">Nothing yet.</p>
                ) : (
                  <ul className="divide-y rounded-xl border">
                    {data.recent.map((e) => (
                      <li key={e.id} className="flex items-baseline justify-between gap-3 px-4 py-2.5 text-[13px]">
                        <span className="min-w-0 truncate">
                          <span className="font-semibold">{actorLabel(e.actor, e.actorName)}</span>{' '}
                          <span className="text-muted-foreground">{actionLabel(e.action)}</span>
                        </span>
                        <span className="shrink-0 text-[12px] text-faint">{formatRelative(e.at)}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </Section>
              </div>
              <div className="min-w-0">
              <Section title="This instance" desc="What applications need to sign people in with Hayat.">
                <div className="rounded-xl border p-4">
                  <DetailList
                    items={[
                      ['Issuer', <CopyField key="i" value={data.issuer} />],
                      ['Discovery', <CopyField key="d" value={`${data.issuer}/.well-known/openid-configuration`} />],
                      ['Region', data.region],
                      ['Version', data.version],
                    ]}
                  />
                </div>
              </Section>
              </div>
            </div>
          </>
        ) : null}
      </div>
    </>
  )
}
