import { Link } from 'react-router'
import { FolderTree, Plus } from 'lucide-react'
import { Button } from '@/shared/ui/button'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import { PageTabs } from '@/shared/ui/page/PageTabs'

/** The header and the All roles / By service tabs both roles views share. */
export function RolesHeader() {
  return (
    <>
      <PageHeader
        title="Roles"
        desc="A role is a bundle of things a person may do, taken from one or more services. It does nothing until you grant it to a person or group on a node of the resource tree."
        action={
          <>
            <Button size="lg" variant="outline" asChild>
              <Link to="/resource-tree">
                <FolderTree /> Grant a role
              </Link>
            </Button>
            <Button size="lg" asChild>
              <Link to="/roles/new">
                <Plus /> Create role
              </Link>
            </Button>
          </>
        }
      />
      <PageTabs
        tabs={[
          { to: '/roles', label: 'All roles', end: true },
          { to: '/roles/by-service', label: 'By service' },
        ]}
      />
    </>
  )
}
