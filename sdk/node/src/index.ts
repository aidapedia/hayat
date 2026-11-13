/**
 * Node client for Hayat's service API.
 *
 *   const hayat = await Hayat.create({
 *     apiUrl: 'http://hayat-api:7443', issuer: 'https://auth.example.com',
 *     clientId: process.env.HAYAT_CLIENT_ID!, clientSecret: process.env.HAYAT_CLIENT_SECRET!,
 *   })
 *   hayat.watch()                                 // keep the decision cache honest
 *   await hayat.check(userId, 'billing.ledger.view', ref('billing/ledger', id))
 *
 * It owns the client credentials token, a short decision cache emptied per
 * tenant by the Watch stream, read-your-writes after a resource write, and
 * offline verification of user access tokens.
 */
import { Code, ConnectError, createClient, type Client, type Interceptor } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-node'
import { createRemoteJWKSet, jwtVerify, type JWTPayload } from 'jose'
import {
  AuthzService,
  DirectoryService,
  SchemaService,
  TreeService,
  WatchService,
  type Consistency,
  type ResourceRef,
} from './gen/hayat/authz/v1/authz_pb.js'

export * from './gen/hayat/authz/v1/authz_pb.js'

export type HayatConfig = {
  /** The api role, e.g. http://hayat-api:7443. */
  apiUrl: string
  /** The auth role's public URL; tokens and keys are discovered from it. */
  issuer: string
  clientId: string
  clientSecret: string
  /** How long a decision is reused, in ms. Default 10 000; 0 turns the cache off. */
  cacheTtlMs?: number
}

export type Resource = { type?: string; externalId?: string; nodeId?: string }

/** Names one of your resources, or a tenant with ref('hayat/tenant', tenantId). */
export const ref = (type: string, externalId: string): Resource => ({ type, externalId })

/** What a Hayat access token says about the person holding it. Never what they may do. */
export type UserClaims = JWTPayload & { sid?: string; client_id?: string }

const GLOBAL = '00000000-0000-0000-0000-000000000000'

type Discovery = { issuer: string; token_endpoint: string; jwks_uri: string }

export class Hayat {
  readonly authz: Client<typeof AuthzService>
  readonly tree: Client<typeof TreeService>
  readonly directory: Client<typeof DirectoryService>
  readonly schema: Client<typeof SchemaService>
  private readonly watcher: Client<typeof WatchService>

  private token = ''
  private tokenExpires = 0
  private tokenPending: Promise<string> | null = null
  private cache = new Map<string, { allowed: boolean; tenant: string; expires: number }>()
  private lastWrite = { rev: 0n, at: 0 }
  private readonly jwks: ReturnType<typeof createRemoteJWKSet>

  private constructor(
    private readonly cfg: HayatConfig,
    private readonly discovery: Discovery,
  ) {
    const auth: Interceptor = (next) => async (req) => {
      req.header.set('Authorization', `Bearer ${await this.accessToken()}`)
      return next(req)
    }
    const transport = createConnectTransport({ baseUrl: cfg.apiUrl.replace(/\/$/, ''), httpVersion: '1.1', interceptors: [auth] })
    this.authz = createClient(AuthzService, transport)
    this.tree = createClient(TreeService, transport)
    this.directory = createClient(DirectoryService, transport)
    this.schema = createClient(SchemaService, transport)
    this.watcher = createClient(WatchService, transport)
    this.jwks = createRemoteJWKSet(new URL(discovery.jwks_uri))
  }

  static async create(cfg: HayatConfig): Promise<Hayat> {
    const res = await fetch(`${cfg.issuer.replace(/\/$/, '')}/.well-known/openid-configuration`)
    if (!res.ok) throw new Error(`hayat: discovery answered ${res.status}`)
    return new Hayat({ cacheTtlMs: 10_000, ...cfg }, (await res.json()) as Discovery)
  }

  /** A client credentials token, refreshed 30 s before it expires. */
  private async accessToken(): Promise<string> {
    if (this.token && Date.now() < this.tokenExpires - 30_000) return this.token
    this.tokenPending ??= (async () => {
      try {
        const res = await fetch(this.discovery.token_endpoint, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
            Authorization: 'Basic ' + Buffer.from(`${encodeURIComponent(this.cfg.clientId)}:${encodeURIComponent(this.cfg.clientSecret)}`).toString('base64'),
          },
          body: new URLSearchParams({ grant_type: 'client_credentials' }),
        })
        if (!res.ok) throw new Error(`hayat: token endpoint answered ${res.status}`)
        const body = (await res.json()) as { access_token: string; expires_in: number }
        this.token = body.access_token
        this.tokenExpires = Date.now() + body.expires_in * 1000
        return this.token
      } finally {
        this.tokenPending = null
      }
    })()
    return this.tokenPending
  }

  private consistency(): Consistency | undefined {
    // For a moment after a write, ask for at least the revision it produced.
    if (Date.now() - this.lastWrite.at < 2000) return { $typeName: 'hayat.authz.v1.Consistency', atLeastRev: this.lastWrite.rev, fullyConsistent: false }
    return undefined
  }

  private refOf(r: Resource): ResourceRef {
    return { $typeName: 'hayat.authz.v1.ResourceRef', type: r.type ?? '', externalId: r.externalId ?? '', nodeId: r.nodeId ?? '' }
  }

  /** Whether a user holds a permission on a resource. An unknown resource is a no. */
  async check(userId: string, permission: string, resource: Resource): Promise<boolean> {
    const key = `${userId}\0${permission}\0${resource.type}\0${resource.externalId}\0${resource.nodeId}`
    const hit = this.cache.get(key)
    if (hit && hit.expires > Date.now()) return hit.allowed
    try {
      const res = await this.authz.check({ userId, permission, resource: this.refOf(resource), consistency: this.consistency() })
      if (this.cfg.cacheTtlMs) {
        if (this.cache.size > 100_000) this.cache.clear()
        this.cache.set(key, { allowed: res.allowed, tenant: res.tenantId, expires: Date.now() + this.cfg.cacheTtlMs })
      }
      return res.allowed
    } catch (err) {
      if (err instanceof ConnectError && err.code === Code.NotFound) return false
      throw err
    }
  }

  /** Mirror one of your resources into the tree under parent. */
  async upsertResource(resource: Resource, name: string, parent: Resource, ownerUserId = '') {
    const res = await this.tree.upsertResource({
      type: resource.type ?? '',
      externalId: resource.externalId ?? '',
      name,
      parent: this.refOf(parent),
      ownerUserId,
    })
    this.wrote(res.tenantId, res.rev)
    return res.resource
  }

  async deleteResource(resource: Resource) {
    const res = await this.tree.deleteResource({ type: resource.type ?? '', externalId: resource.externalId ?? '' })
    this.wrote(res.tenantId, res.rev)
  }

  private wrote(tenant: string, rev: bigint) {
    this.lastWrite = { rev, at: Date.now() }
    this.dropTenant(tenant)
  }

  /** Ids of resources of one type where the user holds permission. */
  async lookup(userId: string, permission: string, type: string, tenantId = ''): Promise<string[]> {
    const out: string[] = []
    let pageToken = ''
    do {
      const res = await this.authz.lookupResources({ userId, permission, type, tenantId, pageSize: 500, pageToken })
      out.push(...res.resources.map((r) => r.externalId))
      pageToken = res.nextPageToken
    } while (pageToken)
    return out
  }

  /** Streams policy changes and drops cached decisions, reconnecting until the signal aborts. */
  async watch(signal?: AbortSignal): Promise<void> {
    let cursor = ''
    let backoff = 1000
    while (!signal?.aborted) {
      try {
        for await (const res of this.watcher.watch({ cursor }, { signal })) {
          const e = res.event
          if (!e) continue
          cursor = e.id
          if (e.tenantId === GLOBAL) this.cache.clear()
          else this.dropTenant(e.tenantId)
          backoff = 1000
        }
      } catch {
        /* reconnect below */
      }
      this.cache.clear() // whatever happened while disconnected is unknown
      if (signal?.aborted) return
      await new Promise((r) => setTimeout(r, backoff))
      backoff = Math.min(backoff * 2, 30_000)
    }
  }

  private dropTenant(tenant: string) {
    for (const [k, v] of this.cache) if (v.tenant === tenant) this.cache.delete(k)
  }

  /**
   * Verify a user's access token offline: signature, issuer, expiry, and that
   * it was issued for `audience`. A sign-in token's `aud` names its client and
   * the client's service, so pass your service's namespace ("billing") to
   * accept any of its sign-in clients, or one client's id to accept only that
   * client. Leave it out to accept any.
   */
  async verifyAccessToken(token: string, audience?: string): Promise<UserClaims> {
    const { payload } = await jwtVerify(token, this.jwks, { issuer: this.discovery.issuer })
    const claims = payload as UserClaims
    if (claims.sub?.startsWith('client:')) throw new Error('hayat: a service token, not a person')
    if (audience) {
      const aud = Array.isArray(claims.aud) ? claims.aud : claims.aud ? [claims.aud] : []
      if (claims.client_id !== audience && !aud.includes(audience)) throw new Error('hayat: token was issued for another service or client')
    }
    return claims
  }

  /** Express/Connect-style middleware: verifies the bearer token and sets req.hayatUser. */
  middleware(audience?: string) {
    return async (req: { headers: Record<string, string | string[] | undefined>; hayatUser?: UserClaims }, res: { statusCode: number; end: (b?: string) => void }, next: () => void) => {
      const header = req.headers.authorization
      const token = typeof header === 'string' && header.startsWith('Bearer ') ? header.slice(7) : ''
      try {
        if (!token) throw new Error('no token')
        req.hayatUser = await this.verifyAccessToken(token, audience)
        next()
      } catch {
        res.statusCode = 401
        res.end('sign in required')
      }
    }
  }
}
