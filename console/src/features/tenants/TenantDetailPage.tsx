import { Link, Navigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { FolderTree } from 'lucide-react'
import { TenantService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { useMe } from '@/features/auth'
import { TenantSignIn } from '@/features/sign-in'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDate } from '@/shared/lib/datetime'
import { ErrorState, LoadingState, LogoTile, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ImageUploadTile } from '@/shared/ui/image-upload'
import { Narrow, PageHeader, Section } from '@/shared/ui/page/PageHeader'
import { PageTabs, type PageTab } from '@/shared/ui/page/PageTabs'
import { TenantMembers } from './components/TenantMembers'
import { TenantSettings } from './components/TenantSettings'

/**
 * One tenant: its logo, name and facts up top, then a tab per part (each an
 * address): /tenants/:id (members), /sign-in, /settings. Whoever may manage
 * the tenant changes its logo by clicking it, and sees the company sign-in
 * tab. The actions shown are the ones the server would allow; it checks each.
 */
export default function TenantDetailPage() {
  const { id = '', tab = '' } = useParams()
  const { isAdmin, me } = useMe()
  const tenants = useClient(TenantService)
  const { data, error, loading, reload } = useQuery(() => tenants.getTenant({ id }), [tenants, id])

  if (error) return <ErrorState message={error} onRetry={reload} />
  if (loading && !data) return <LoadingState />
  if (!data?.tenant) return null
  const t = data.tenant
  const base = `/tenants/${t.id}`

  const canEdit = data.myPermissions.includes('hayat.tenant.manage')
  const canManageMembers = data.myPermissions.includes('hayat.members.manage')
  const canTransfer = isAdmin || t.ownerId === me.user?.id
  const settingsLabel = canEdit || isAdmin ? 'Settings' : 'Details'

  const tabs: PageTab[] = [
    { to: base, label: `Members · ${t.memberCount}`, end: true },
    ...(canEdit ? [{ to: `${base}/sign-in`, label: 'Company sign-in' }] : []),
    { to: `${base}/settings`, label: settingsLabel },
  ]
  const known = ['', 'settings', ...(canEdit ? ['sign-in'] : [])]
  if (!known.includes(tab)) return <Navigate to={base} replace />

  const setLogo = canEdit
    ? async (logo: Uint8Array) => {
        await tenants.setTenantLogo({ id: t.id, data: logo })
        toast.success('Logo updated')
        reload()
      }
    : undefined

  let body
  switch (tab) {
    case 'sign-in':
      body = (
        <Narrow width={880}>
          <Section title="Company sign-in" desc="Let people at your company's email domains sign in with your own identity provider, or require it.">
          <TenantSignIn tenantId={t.id} />
        </Section>
        </Narrow>
      )
      break
    case 'settings':
      body = <TenantSettings tenant={t} canEdit={canEdit} canDelete={isAdmin} onChanged={reload} />
      break
    default:
      body = <TenantMembers tenant={t} canManageMembers={canManageMembers} canTransfer={canTransfer} linkPeople={isAdmin} onChanged={reload} />
  }

  return (
    <>
      <PageHeader
        title={t.name}
        crumbs={[{ label: t.name, to: tab ? base : undefined }]}
        media={
          <ImageUploadTile label="Logo" rounded="rounded-[16px]" onUpload={setLogo}>
            <LogoTile name={t.name} src={t.logoUrl} size={64} className="rounded-[16px]" />
          </ImageUploadTile>
        }
        meta={<Pill className="font-mono normal-case">{t.slug}</Pill>}
        desc={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>
              {t.memberCount} {t.memberCount === 1 ? 'member' : 'members'}
            </span>
            <Dot />
            <span>Home region {t.region}</span>
            <Dot />
            <span>Since {formatDate(t.createdAt)}</span>
          </span>
        }
        action={
          <Button variant="outline" size="lg" asChild>
            <Link to={`/resource-tree/${t.id}`}>
              <FolderTree /> Resource tree
            </Link>
          </Button>
        }
      />
      <PageTabs tabs={tabs} />
      <div className="mt-7">{body}</div>
    </>
  )
}

function Dot() {
  return (
    <span aria-hidden="true" className="text-faint">
      ·
    </span>
  )
}
