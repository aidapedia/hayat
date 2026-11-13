import { useState, type KeyboardEvent } from 'react'
import { X } from 'lucide-react'
import { Button } from '@/shared/ui/button'
import { Input } from '@/shared/ui/input'

/**
 * A list of URIs, one per row: each can be removed, and a new one is added
 * from the field at the bottom (Add, or Enter). Duplicates and blanks are
 * ignored. The server checks each URI when the form is saved.
 */
export function UriListInput({
  id,
  value,
  onChange,
  placeholder,
  disabled,
}: {
  id?: string
  value: string[]
  onChange: (next: string[]) => void
  placeholder?: string
  disabled?: boolean
}) {
  const [draft, setDraft] = useState('')

  const add = () => {
    const v = draft.trim()
    if (!v || value.includes(v)) return
    onChange([...value, v])
    setDraft('')
  }
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      add()
    }
  }

  return (
    <div className="overflow-hidden rounded-[10px] border">
      {value.map((uri) => (
        <div key={uri} className="flex min-h-9 items-center gap-2 border-b py-1 pr-1 pl-3">
          <span className="min-w-0 flex-1 truncate font-mono text-[12px]">{uri}</span>
          {!disabled && (
            <Button type="button" variant="ghost" size="icon-sm" aria-label={`Remove ${uri}`} onClick={() => onChange(value.filter((u) => u !== uri))}>
              <X />
            </Button>
          )}
        </div>
      ))}
      {!disabled && (
        <div className="flex gap-1.5 bg-muted/50 p-1.5">
          <Input
            id={id}
            type="url"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onKey}
            placeholder={placeholder}
            className="h-8 bg-background font-mono text-[12px]"
          />
          <Button type="button" variant="outline" className="h-8" onClick={add} disabled={!draft.trim()}>
            Add
          </Button>
        </div>
      )}
      {disabled && value.length === 0 && <div className="px-3 py-2 text-[12px] text-faint">None</div>}
    </div>
  )
}
