import { useState, type ReactNode } from 'react'
import { ChevronsUpDownIcon } from 'lucide-react'
import { cn } from '@/shared/lib/utils'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/shared/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/shared/ui/popover'

export type ComboboxOption = {
  value: string
  label: string
  /** Muted text on the right of the row: an offset, a code, an email. */
  hint?: string
  /** Extra words the search matches on, besides the label, hint and value. */
  keywords?: string[]
  icon?: ReactNode
}

const CLEAR = '\u0000clear'

/** A list this long gets a search box; a shorter one is quicker to scan. */
const SEARCH_FROM = 8

/** Every word typed must appear somewhere in the row — fuzzy matching is too loose for long lists. */
function contains(value: string, search: string, keywords?: string[]) {
  const hay = [value, ...(keywords ?? [])].join(' ').toLowerCase()
  return search.toLowerCase().split(/\s+/).every((w) => hay.includes(w)) ? 1 : 0
}

const triggerVariants = {
  default:
    'h-8 rounded-lg border border-input bg-transparent pr-2 pl-2.5 text-sm dark:bg-input/30 dark:hover:bg-input/50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20',
  ghost: 'h-7 rounded-md px-2 text-[12px] hover:bg-muted hover:text-foreground',
}

/**
 * The console's one single-choice list: every "pick one of these" uses it, from
 * Person/Group to the 400 time zones. Long lists get a search box.
 * `variant="ghost"` is the bare switcher for a toolbar or table footer.
 */
export function Combobox({
  id,
  value,
  onChange,
  options,
  placeholder = 'Select…',
  searchPlaceholder = 'Search…',
  empty = 'Nothing found.',
  searchable = options.length >= SEARCH_FROM,
  clearLabel,
  renderValue,
  variant = 'default',
  side,
  disabled,
  invalid,
  className,
  contentClassName,
  'aria-label': ariaLabel,
}: {
  id?: string
  value: string
  onChange: (value: string) => void
  options: ComboboxOption[]
  placeholder?: string
  searchPlaceholder?: string
  empty?: string
  /** Show the search box (default: when the list has 8 or more options). */
  searchable?: boolean
  /** Offer a first row that empties the value, labelled with this text. */
  clearLabel?: string
  /** What the closed trigger shows for the selected option (default: its icon and label). */
  renderValue?: (option: ComboboxOption) => ReactNode
  variant?: keyof typeof triggerVariants
  side?: 'top' | 'bottom'
  disabled?: boolean
  invalid?: boolean
  className?: string
  contentClassName?: string
  'aria-label'?: string
}) {
  const [open, setOpen] = useState(false)
  const selected = options.find((o) => o.value === value)
  const pick = (v: string) => {
    onChange(v)
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          id={id}
          type="button"
          role="combobox"
          aria-expanded={open}
          aria-label={ariaLabel}
          aria-invalid={invalid || undefined}
          disabled={disabled}
          className={cn(
            'flex w-full min-w-0 items-center justify-between gap-1.5 text-left whitespace-nowrap transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50',
            triggerVariants[variant],
            className,
          )}
        >
          <span className={cn('flex min-w-0 items-center gap-1.5', !selected && !value && 'text-muted-foreground')}>
            {selected ? (
              renderValue ? (
                renderValue(selected)
              ) : (
                <>
                  {selected.icon}
                  <span className="truncate">{selected.label}</span>
                </>
              )
            ) : (
              <span className="truncate">{value || placeholder}</span>
            )}
          </span>
          <ChevronsUpDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        side={side}
        className={cn('w-(--radix-popover-trigger-width) min-w-48 p-0', searchable && 'min-w-64', contentClassName)}
      >
        <Command filter={contains}>
          {searchable && <CommandInput placeholder={searchPlaceholder} />}
          <CommandList>
            <CommandEmpty className="py-4 text-[13px] text-muted-foreground">{empty}</CommandEmpty>
            <CommandGroup>
              {clearLabel && (
                <CommandItem value={CLEAR} keywords={[clearLabel]} onSelect={() => pick('')} className="text-muted-foreground" data-checked={!value}>
                  <span className="flex-1">{clearLabel}</span>
                </CommandItem>
              )}
              {options.map((o) => (
                <CommandItem
                  key={o.value}
                  value={o.value}
                  keywords={[o.label, ...(o.hint ? [o.hint] : []), ...(o.keywords ?? [])]}
                  data-checked={o.value === value}
                  onSelect={() => pick(o.value)}
                >
                  {o.icon}
                  <span className="min-w-0 flex-1 truncate">{o.label}</span>
                  {o.hint && <span className="shrink-0 pl-2 text-xs text-muted-foreground">{o.hint}</span>}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
