import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { TenantService, type Tenant } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { formatDateTime } from '@/shared/lib/datetime'
import { CopyField, DetailList, Field, LogoTile } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { ImageUpload } from '@/shared/ui/image-upload'
import { Input } from '@/shared/ui/input'
import { Narrow, Section } from '@/shared/ui/page/PageHeader'

/**
 * The tenant's profile (logo and name, for whoever holds
 * hayat.tenant.manage), its read-only details, and deleting it (instance
 * admins only).
 */
export function TenantSettings({ tenant: t, canEdit, canDelete, onChanged }: { tenant: Tenant; canEdit: boolean; canDelete: boolean; onChanged: () => void }) {
  const tenants = useClient(TenantService)
  const navigate = useNavigate()
  const [name, setName] = useState(t.name)
  const [busy, setBusy] = useState(false)
  const [deleting, setDeleting] = useState(false)
  useEffect(() => setName(t.name), [t.name])

  const setLogo = async (data: Uint8Array) => {
    await tenants.setTenantLogo({ id: t.id, data })
    toast.success(data.length ? 'Logo updated' : 'Logo removed')
    onChanged()
  }

  const rename = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await tenants.renameTenant({ id: t.id, name: name.trim() })
      toast.success('Tenant renamed')
      onChanged()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Narrow width={760}>
      {canEdit && (
        <Section title="Profile" desc="Every service that uses Hayat shows this name and logo for the tenant.">
          <div className="overflow-hidden rounded-xl border">
            <div className="p-5">
              <ImageUpload
                label="Logo"
                hint="A square image works best. PNG, JPEG or WebP, up to 5 MB."
                preview={<LogoTile name={t.name} src={t.logoUrl} size={64} className="rounded-[14px]" />}
                hasImage={!!t.logoUrl}
                onUpload={setLogo}
                onRemove={() => setLogo(new Uint8Array())}
              />
            </div>
            <form onSubmit={rename} className="flex items-end gap-3 border-t p-5 max-[600px]:flex-col max-[600px]:items-stretch">
              <div className="min-w-0 flex-1">
                <Field label="Name" htmlFor="t-name">
                  <Input id="t-name" value={name} onChange={(e) => setName(e.target.value)} required />
                </Field>
              </div>
              <Button type="submit" disabled={busy || !name.trim() || name.trim() === t.name}>
                {busy ? 'Saving…' : 'Save name'}
              </Button>
            </form>
          </div>
        </Section>
      )}

      <Section title="Details" desc="The slug and region are set when the tenant is created and never change.">
        <div className="rounded-xl border p-5">
          <DetailList
            items={[
              ['Tenant id', <CopyField key="id" value={t.id} />],
              ['Slug', <span key="s" className="font-mono text-[12px]">{t.slug}</span>],
              ['Home region', t.region],
              ['Created', formatDateTime(t.createdAt)],
              ['Updated', formatDateTime(t.updatedAt)],
            ]}
          />
        </div>
      </Section>

      {canDelete && (
        <Section title="Danger zone">
          <div className="flex items-center gap-4 rounded-xl border border-destructive/40 p-5 max-[600px]:flex-col max-[600px]:items-stretch">
            <div className="min-w-0 flex-1">
              <div className="text-[13px] font-semibold">Delete this tenant</div>
              <p className="mt-0.5 text-[12px] text-muted-foreground">
                The tenant and its membership list are deleted; accounts are kept. Refused while any service still files resources under it.
              </p>
            </div>
            <Button variant="destructive" onClick={() => setDeleting(true)}>
              Delete tenant
            </Button>
          </div>
        </Section>
      )}

      <ConfirmDialog
        open={deleting}
        onOpenChange={setDeleting}
        title={`Delete ${t.name}?`}
        body="The tenant and its membership list are deleted. Accounts are kept. This is refused while any service still files resources under the tenant."
        confirm="Delete tenant"
        destructive
        typeToConfirm={t.slug}
        onConfirm={async () => {
          await tenants.deleteTenant({ id: t.id })
          toast.success(`${t.name} deleted`)
          navigate('/tenants')
        }}
      />
    </Narrow>
  )
}
