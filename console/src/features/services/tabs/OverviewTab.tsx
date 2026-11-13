import { Link } from 'react-router'
import { AppWindow, Server } from 'lucide-react'
import { typeLabel } from '@/features/roles/lib/roles'
import { CatalogService, RegistryService, type GetServiceResponse } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDate } from '@/shared/lib/datetime'
import { DetailList, PersonRow, StatusBadge } from '@/shared/ui/bits'
import { Section } from '@/shared/ui/page/PageHeader'
import { RequestCard } from '../components/RequestCard'
import { activeContract, clientKind, ownPending, readManifest, returnsTo } from '../lib'

function Stat({ value, label }: { value: number; label: string }) {
  return (
    <div className="rounded-xl border px-4 py-3.5">
      <div className="text-[24px] font-semibold tabular-nums">{value}</div>
      <div className="mt-0.5 text-[12px] text-muted-foreground">{label}</div>
    </div>
  )
}

function MoreLink({ to, children }: { to: string; children: string }) {
  return (
    <Link to={to} className="text-[12px] font-semibold whitespace-nowrap text-primary hover:underline">
      {children} →
    </Link>
  )
}

/** A service at a glance: what waits on it, what it declares, how it reaches Hayat, who owns it. */
export function OverviewTab({ data, onChanged }: { data: GetServiceResponse; onChanged: () => unknown }) {
  const s = data.service!
  const base = `/services/${s.id}`
  const registry = useClient(RegistryService)
  const catalog = useClient(CatalogService)
  const { data: clients } = useQuery(() => registry.listServiceClients({ serviceId: s.id }), [registry, s.id])
  const { data: types } = useQuery(() => catalog.getCatalog({}).then((c) => c.types.filter((t) => t.service === s.id)), [catalog, s.id])
  const active = activeContract(data.contracts)
  const manifest = readManifest(active?.manifestJson)
  const pending = ownPending(data.contracts)

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_minmax(280px,340px)] gap-x-10 max-[1000px]:grid-cols-1">
      <div className="min-w-0">
        {(data.incoming.length > 0 || pending) && (
          <section className="mb-9 flex flex-col gap-3">
            {data.incoming.map((c) => (
              <RequestCard key={`${c.serviceId}-${c.version}`} c={c} incoming onChanged={onChanged} />
            ))}
            {pending && <RequestCard c={pending} incoming={false} onChanged={onChanged} />}
          </section>
        )}

        {!s.builtin && (
          <Section
            title={active ? `What version ${active.version} declares` : 'Contract'}
            desc={active ? `Published by ${active.submittedBy} · ${formatDate(active.decidedAt ?? active.submittedAt)}. Contracts only add.` : 'No version is live yet.'}
            action={<MoreLink to={`${base}/contract`}>{active ? 'All versions' : 'Publish one'}</MoreLink>}
          >
            {manifest ? (
              <div className="grid grid-cols-3 gap-3 max-[600px]:grid-cols-1">
                <Stat value={manifest.resource_types?.length ?? 0} label="Resource types" />
                <Stat value={manifest.permissions?.length ?? 0} label="Permissions" />
                <Stat value={manifest.default_roles?.length ?? 0} label="Suggested roles" />
              </div>
            ) : (
              <p className="rounded-xl border border-dashed px-4 py-6 text-center text-[12px] text-muted-foreground">
                Publish a contract to declare this service's types, permissions and roles.
              </p>
            )}
          </Section>
        )}

        {types && types.length > 0 && (
          <Section title="Where it puts things in the tree" desc="The node types it adds. A role granted on one reaches everything filed below it.">
            <div className="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-3">
              {types.map((t) => (
                <div key={t.key} className="flex flex-col gap-1.5 rounded-xl border px-3.5 py-3">
                  <span className="text-[13px] font-semibold">{typeLabel(t.key)}</span>
                  <span className="font-mono text-[11px] text-muted-foreground">{t.key}</span>
                  {t.description && <span className="text-[12px] text-muted-foreground">{t.description}</span>}
                  <span className="text-[11.5px] text-muted-foreground">Lives under: {t.parents.length ? t.parents.map(typeLabel).join(', ') : 'nothing (a root)'}</span>
                </div>
              ))}
            </div>
          </Section>
        )}

        <Section
          title="How it reaches Hayat"
          desc={s.builtin ? 'The console signs in through these.' : 'Its API credential, and the OAuth clients people sign in through.'}
          action={<MoreLink to={`${base}/clients`}>Manage clients</MoreLink>}
        >
          <div className="divide-y rounded-xl border">
            {!s.builtin && (
              <Link to={`${base}/clients`} className="flex items-center gap-3 px-4 py-3 hover:bg-accent/50">
                <span className="inline-flex size-8 items-center justify-center rounded-[9px] bg-muted text-muted-foreground">
                  <Server className="size-4" />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="text-[13px] font-semibold">API credential</div>
                  <div className="truncate font-mono text-[12px] text-faint">{s.clientId}</div>
                </div>
                <span className="text-[12px] text-muted-foreground">Client credentials</span>
              </Link>
            )}
            {(clients?.clients ?? []).map((c) => (
              <Link key={c.id} to={`${base}/clients/${c.id}`} className="flex items-center gap-3 px-4 py-3 hover:bg-accent/50">
                <span className="inline-flex size-8 items-center justify-center rounded-[9px] bg-primary/10 text-primary">
                  <AppWindow className="size-4" />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="text-[13px] font-semibold">{c.name}</div>
                  <div className="truncate font-mono text-[12px] text-faint">{returnsTo(c)}</div>
                </div>
                {c.disabled ? <StatusBadge status="disabled" /> : <span className="text-[12px] text-muted-foreground">{clientKind(c).label}</span>}
              </Link>
            ))}
            {clients && clients.clients.length === 0 && <p className="px-4 py-3 text-[12px] text-faint">No sign-in clients yet.</p>}
          </div>
        </Section>
      </div>

      <div className="min-w-0 max-[1000px]:mt-9">
        {!s.builtin && (
          <Section title="Owners" desc="They publish contracts and manage clients." action={<MoreLink to={`${base}/owners`}>Manage owners</MoreLink>}>
            <div className="divide-y rounded-xl border">
              {s.owners.map((o) => (
                <div key={o.id} className="px-3 py-2.5">
                  <PersonRow name={o.name} email={o.email} />
                </div>
              ))}
            </div>
          </Section>
        )}
        <Section title="Details">
          <div className="rounded-xl border p-4">
            <DetailList
              stacked
              items={[
                ['Namespace', <span key="ns" className="font-mono">{s.id}</span>],
                ['Types and permissions', <span key="q" className="font-mono">{`${s.id}/…  ·  ${s.id}.…`}</span>],
                ...(s.builtin ? [] : ([['Registered', formatDate(s.createdAt)]] as [string, string][])),
              ]}
            />
          </div>
        </Section>
      </div>
    </div>
  )
}
