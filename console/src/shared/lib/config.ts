// Where Hayat is and which client the console is. Served by the admin role at
// /config.json, so one console image works against any deployment.
export type AppConfig = {
  issuer: string
  clientId: string
  region: string
  version: string
}

export async function loadConfig(): Promise<AppConfig> {
  const res = await fetch('/config.json', { cache: 'no-store' })
  if (!res.ok) throw new Error(`The admin API answered ${res.status} for /config.json. Is the admin role running?`)
  return res.json()
}
