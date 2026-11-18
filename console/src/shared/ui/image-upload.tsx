import { useRef, useState, type ReactNode } from 'react'
import { Camera, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { errorMessage } from '@/shared/api/client'
import { Button } from '@/shared/ui/button'
import { cn } from '@/shared/lib/utils'

const MAX_BYTES = 5 * 1024 * 1024

/**
 * Picking an image file and handing its bytes on: the hidden input, the 5 MB
 * check, the busy state and the error toast. Hayat checks, crops and
 * re-encodes the image itself, so this only reads the file.
 */
function useImagePicker(label: string, onUpload: (data: Uint8Array) => Promise<void>) {
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
    if (file.size > MAX_BYTES) {
      toast.error('That image is over 5 MB. Pick a smaller one.')
      return
    }
    void run('upload', async () => onUpload(new Uint8Array(await file.arrayBuffer())))
  }

  const field = (
    <input
      ref={input}
      type="file"
      accept="image/png,image/jpeg,image/webp"
      className="sr-only"
      tabIndex={-1}
      aria-label={`Upload ${label.toLowerCase()}`}
      onChange={(e) => pick(e.target.files?.[0])}
    />
  )
  return { field, busy, run, open: () => input.current?.click() }
}

/**
 * An image someone can upload, replace or remove: an avatar, a tenant's or a
 * service's logo. `onUpload` gets its bytes; `onRemove` clears it.
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
  const { field, busy, run, open } = useImagePicker(label, onUpload)
  return (
    <div className="flex items-center gap-4">
      {preview}
      <div className="min-w-0 flex-1">
        <div className="text-[13px] font-semibold">{label}</div>
        <div className="text-[12px] text-muted-foreground">{hint}</div>
      </div>
      {field}
      <div className="flex shrink-0 gap-2">
        {hasImage && (
          <Button type="button" variant="ghost" disabled={disabled || busy !== null} onClick={() => run('remove', onRemove)}>
            {busy === 'remove' && <Loader2 className="animate-spin" />}
            Remove
          </Button>
        )}
        <Button type="button" variant="outline" disabled={disabled || busy !== null} onClick={open}>
          {busy === 'upload' && <Loader2 className="animate-spin" />}
          {hasImage ? 'Replace' : 'Upload'}
        </Button>
      </div>
    </div>
  )
}

/**
 * A logo or avatar that is its own upload button: hovering shows a camera,
 * clicking picks a file. `rounded` matches the tile it wraps. Without
 * `onUpload` it is just the tile.
 */
export function ImageUploadTile({
  label,
  rounded = 'rounded-[10px]',
  onUpload,
  children,
}: {
  label: string
  rounded?: string
  onUpload?: (data: Uint8Array) => Promise<void>
  children: ReactNode
}) {
  const { field, busy, open } = useImagePicker(label, onUpload ?? (async () => {}))
  if (!onUpload) return <>{children}</>
  return (
    <>
    <button
      type="button"
      onClick={open}
      disabled={busy !== null}
      title={`Change ${label.toLowerCase()}`}
      aria-label={`Change ${label.toLowerCase()}`}
      className={cn('group relative block cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2', rounded)}
    >
      {children}
      <span
        className={cn(
          'absolute inset-0 flex items-center justify-center bg-black/55 text-white transition-opacity',
          rounded,
          busy ? 'opacity-100' : 'opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100',
        )}
      >
        {busy ? <Loader2 className="size-5 animate-spin" /> : <Camera className="size-5" />}
      </span>
    </button>
    {field}
    </>
  )
}
