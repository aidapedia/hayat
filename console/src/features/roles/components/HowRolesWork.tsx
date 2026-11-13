import { useState } from 'react'
import { Button } from '@/shared/ui/button'

const KEY = 'hayat.roles.how-hidden'

const STEPS = [
  ['Bundle permissions', 'Pick what someone may do. Permissions come from services: Hayat itself, and every service that publishes a contract.'],
  ['Grant it on a node', 'Give the role to a person or a group on a tenant, a group, or one resource.'],
  ['It flows down', 'A grant reaches everything below that node, unless the grant is set not to inherit.'],
]

function readHidden() {
  try {
    return localStorage.getItem(KEY) === '1'
  } catch {
    return false
  }
}

/** Three steps from a role to a person. Hidden once someone says they've got it; remembered in this browser only. */
export function HowRolesWork() {
  const [hidden, setHidden] = useState(readHidden)
  if (hidden) return null
  const hide = () => {
    setHidden(true)
    try {
      localStorage.setItem(KEY, '1')
    } catch {
      // Storage is blocked; it simply shows again next time.
    }
  }
  return (
    <section aria-labelledby="how-roles" className="mt-6 rounded-2xl border bg-muted/40 px-4 py-3.5">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h2 id="how-roles" className="text-[13px] font-semibold">
          How a role reaches someone
        </h2>
        <Button variant="ghost" size="sm" onClick={hide} className="text-muted-foreground">
          Got it, hide this
        </Button>
      </div>
      <ol className="grid grid-cols-3 gap-3 max-[860px]:grid-cols-1">
        {STEPS.map(([title, body], i) => (
          <li key={title} className="flex items-start gap-3 rounded-xl border bg-background px-3.5 py-3">
            <span className="flex size-[26px] shrink-0 items-center justify-center rounded-full bg-primary/12 text-[12px] font-bold text-primary">{i + 1}</span>
            <span className="flex flex-col gap-0.5">
              <span className="text-[13px] font-semibold">{title}</span>
              <span className="text-[12.5px] text-muted-foreground">{body}</span>
            </span>
          </li>
        ))}
      </ol>
    </section>
  )
}
