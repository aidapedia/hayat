import { useRef, useState, type ReactNode } from 'react'
import { Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { errorMessage } from '@/shared/api/client'
import { Button } from '@/shared/ui/button'

/**
 * An image someone can upload, replace or remove: an avatar, a tenant's or a
 * service's logo. Hayat checks, crops and re-encodes it, so this only reads
 * the file. `onUpload` gets its bytes; `onRemove` clears it.
 */
export function ImageUpload({
  preview,
  label,
  hint = 'PNG, JPEG or WebP, up to 5 MB.',
  hasImage,
  disabled,
  onUpload,
  onRemove,
}: {
  preview: ReactNode
  label: string
  hint?: ReactNode
  hasImage: boolean
  disabled?: boolean
  onUpload: (data: Uint8Array) => Promise<void>
  onRemove: () => Promise<void>
}) {
  const input = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState<'upload' | 'remove' | null>(null)

  const run = async (kind: 'upload' | 'remove', fn: () => Promise<void>) => {
    setBusy(kind)
    try {
      await fn()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(null)
      if (input.current) input.current.value = ''
    }
  }

  const pick = (file: File | undefined) => {
    if (!file) return
    if (file.size > 5 * 1024 * 1024) {
      toast.error('That image is over 5 MB. Pick a smaller one.')
      return
    }
    void run('upload', async () => onUpload(new Uint8Array(await file.arrayBuffer())))
  }

  return (
    <div className="flex items-center gap-4">
      {preview}
      <div className="min-w-0 flex-1">
        <div className="text-[13px] font-semibold">{label}</div>
        <div className="text-[12px] text-muted-foreground">{hint}</div>
      </div>
      <input
        ref={input}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        className="sr-only"
        aria-label={`Upload ${label.toLowerCase()}`}
        onChange={(e) => pick(e.target.files?.[0])}
      />
      <div className="flex shrink-0 gap-2">
        {hasImage && (
          <Button type="button" variant="ghost" disabled={disabled || busy !== null} onClick={() => run('remove', onRemove)}>
            {busy === 'remove' && <Loader2 className="animate-spin" />}
            Remove
          </Button>
        )}
        <Button type="button" variant="outline" disabled={disabled || busy !== null} onClick={() => input.current?.click()}>
          {busy === 'upload' && <Loader2 className="animate-spin" />}
          {hasImage ? 'Replace' : 'Upload'}
        </Button>
      </div>
    </div>
  )
}
