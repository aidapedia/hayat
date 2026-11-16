import { useState } from 'react'
import { Link } from 'react-router'
import { FileCode2, Plus } from 'lucide-react'
import type { GetServiceResponse } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { formatDate } from '@/shared/lib/datetime'
import { cn } from '@/shared/lib/utils'
import { EmptyState, Pill, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'
import { ManifestForm, ManifestYaml } from '../components/ManifestView'
import { RequestCard } from '../components/RequestCard'
import { activeContract, ownPending, readManifest, readSummary, summaryCount } from '../lib'

const statusOrder = ['active', 'pending'] as const

/** A service's contract: what waits on it, every version, and one version read as a form or as YAML. */
export function ContractTab({ data, onChanged }: { data: GetServiceResponse; onChanged: () => unknown }) {
  const s = data.service!
  const contracts = data.contracts
  const active = activeContract(contracts)
  const pending = ownPending(contracts)
  const [picked, setPicked] = useState<number | null>(null)
  const [view, setView] = useState<'form' | 'yaml'>('form')
  const shown = contracts.find((c) => c.version === picked) ?? active ?? contracts[0]
  const manifest = readManifest(shown?.manifestJson)
  const summary = readSummary(shown?.summaryJson)
  const added = summaryCount(summary)

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="flex flex-wrap items-center gap-2 text-[13px] text-muted-foreground">
          {active ? <Pill className="border-success/30 text-success">v{active.version} active</Pill> : <Pill>None active</Pill>}
          {contracts.length} {contracts.length === 1 ? 'version' : 'versions'}. A version you publish goes live at once.
        </p>
        {data.canManage && (
          <div className="flex items-center gap-2.5">
            {pending && <span className="text-[12px] text-faint">Version {pending.version} must be approved or rejected first</span>}
            {pending ? (
              <Button size="lg" disabled>
                <Plus /> Publish a version
              </Button>
            ) : (
              <Button size="lg" asChild>
                <Link to={`/services/${s.id}/contract/publish`}>
                  <Plus /> Publish a version
                </Link>
              </Button>
            )}
          </div>
        )}
      </div>

      {data.incoming.map((c) => (
        <RequestCard key={`${c.serviceId}-${c.version}`} c={c} incoming onChanged={onChanged} />
      ))}
      {pending && <RequestCard c={pending} incoming={false} onChanged={onChanged} />}

      {contracts.length === 0 ? (
        <EmptyState icon={FileCode2} title="No contract yet">
          Publish one here, or from CI through the API's SchemaService with the service's API credential.
        </EmptyState>
      ) : (
        <div className="grid grid-cols-[minmax(260px,340px)_minmax(0,1fr)] gap-7 max-[1000px]:grid-cols-1">
          <section aria-labelledby="versions-h" className="min-w-0">
            <h2 id="versions-h" className="text-[15px] font-semibold tracking-[-0.2px]">
              Versions
            </h2>
            <p className="mt-0.5 mb-3 text-[12px] text-muted-foreground">A version only adds; nothing live can be taken back.</p>
            <div className="divide-y overflow-hidden rounded-xl border">
              {[...contracts]
                .sort((a, b) => b.version - a.version)
                .map((c) => {
                  const on = shown?.version === c.version
                  const n = summaryCount(readSummary(c.summaryJson))
                  return (
                    <button
                      key={c.version}
                      type="button"
                      aria-pressed={on}
                      onClick={() => setPicked(c.version)}
                      className={cn('flex w-full flex-col gap-1.5 px-4 py-3 text-left hover:bg-accent/50', on && 'bg-primary/5 shadow-[inset_3px_0_0_var(--primary)]')}
                    >
                      <span className="flex w-full items-center justify-between gap-2">
                        <span className={cn('text-[13px] font-semibold', !statusOrder.includes(c.status as never) && 'text-muted-foreground')}>Version {c.version}</span>
                        <StatusBadge status={c.status} />
                      </span>
                      {n > 0 && <span className="text-[12px] text-muted-foreground">{n} added</span>}
                      {c.reason && <span className="text-[12px] text-destructive">“{c.reason}”</span>}
                      <span className="text-[11px] text-faint">
                        {c.submittedBy} · {formatDate(c.decidedAt ?? c.submittedAt)}
                      </span>
                    </button>
                  )
                })}
            </div>
          </section>

          {shown && (
            <section aria-labelledby="manifest-h" className="min-w-0">
              <h2 id="manifest-h" className="text-[15px] font-semibold tracking-[-0.2px]">
                Version {shown.version} · {shown.status}
              </h2>
              <p className="mt-0.5 mb-3 text-[12px] text-muted-foreground">{added > 0 ? 'What it added over the version before is marked New.' : 'Everything it declares.'}</p>
              <div className="overflow-hidden rounded-xl border">
                <div className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2">
                  <Tabs value={view} onValueChange={(v) => setView(v as 'form' | 'yaml')}>
                    <TabsList>
                      <TabsTrigger value="form" className="px-3 text-[12px]">
                        Form
                      </TabsTrigger>
                      <TabsTrigger value="yaml" className="px-3 text-[12px]">
                        YAML
                      </TabsTrigger>
                    </TabsList>
                  </Tabs>
                  {added > 0 && <Pill className="border-success/30 text-success">{added} new</Pill>}
                </div>
                {!manifest ? (
                  <pre className="p-4 font-mono text-[11px]">{shown.manifestJson}</pre>
                ) : view === 'form' ? (
                  <ManifestForm manifest={manifest} summary={summary} />
                ) : (
                  <ManifestYaml manifest={manifest} />
                )}
              </div>
            </section>
          )}
        </div>
      )}
    </div>
  )
}
