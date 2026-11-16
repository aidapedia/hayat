import type { ComboboxOption } from '@/shared/ui/combobox'

/** Every IANA zone the browser knows, labelled with its current UTC offset. */
export function timeZoneOptions(): ComboboxOption[] {
  let zones: string[] = []
  try {
    zones = (Intl as unknown as { supportedValuesOf(k: string): string[] }).supportedValuesOf('timeZone')
  } catch {
    /* very old browser: the list stays empty and the current value still shows */
  }
  if (!zones.includes('UTC')) zones = ['UTC', ...zones]
  const now = new Date()
  return zones.map((zone) => {
    const offset = offsetOf(zone, now)
    const city = zone.split('/').pop()!.replace(/_/g, ' ')
    return { value: zone, label: zone.replace(/_/g, ' '), hint: offset, keywords: [city, offset] }
  })
}

function offsetOf(zone: string, at: Date): string {
  try {
    const part = new Intl.DateTimeFormat('en-US', { timeZone: zone, timeZoneName: 'shortOffset' })
      .formatToParts(at)
      .find((p) => p.type === 'timeZoneName')
    return part?.value === 'GMT' ? 'GMT+0' : (part?.value ?? '')
  } catch {
    return ''
  }
}

/**
 * The languages offered for an account. A BCP 47 tag is what the OIDC `locale`
 * claim carries; applications pick their own nearest translation from it.
 */
const LANGUAGE_TAGS = [
  'ar', 'bn', 'cs', 'da', 'de', 'el', 'en', 'en-AU', 'en-GB', 'en-US', 'es', 'es-MX', 'fa', 'fi', 'fil',
  'fr', 'fr-CA', 'he', 'hi', 'hu', 'id', 'it', 'ja', 'jv', 'km', 'ko', 'lo', 'ms', 'my', 'nb', 'nl', 'pl',
  'pt-BR', 'pt-PT', 'ro', 'ru', 'sk', 'su', 'sv', 'sw', 'ta', 'th', 'tr', 'uk', 'ur', 'vi', 'zh-CN', 'zh-HK', 'zh-TW',
]

/** The languages, plus `current` when it was set to a tag outside the list. */
export function languageOptions(current: string): ComboboxOption[] {
  const tags = current && !LANGUAGE_TAGS.includes(current) ? [current, ...LANGUAGE_TAGS] : LANGUAGE_TAGS
  let english: Intl.DisplayNames | null = null
  try {
    english = new Intl.DisplayNames(['en'], { type: 'language' })
  } catch {
    /* fall back to the tag itself */
  }
  return tags
    .map((tag) => {
      const label = safe(() => english?.of(tag)) ?? tag
      const native = safe(() => new Intl.DisplayNames([tag], { type: 'language' }).of(tag))
      return {
        value: tag,
        label,
        hint: tag,
        keywords: native && native !== label ? [native] : [],
      }
    })
    .sort((a, b) => a.label.localeCompare(b.label))
}

function safe<T>(fn: () => T): T | undefined {
  try {
    return fn()
  } catch {
    return undefined
  }
}
