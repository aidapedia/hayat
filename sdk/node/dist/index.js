/**
 * Node client for Hayat's service API.
 *
 *   const hayat = await Hayat.create({
 *     apiUrl: 'http://hayat-api:7443', issuer: 'https://auth.example.com',
 *     clientId: process.env.HAYAT_CLIENT_ID!, clientSecret: process.env.HAYAT_CLIENT_SECRET!,
 *     minHayatVersion: '1.2.0',                   // the newest "Since" among the RPCs you call
 *   })
 *   hayat.watch()                                 // keep the decision cache honest
 *   await hayat.check(userId, 'billing.ledger.view', ref('billing/ledger', id))
 *
 * It owns the client credentials token, a short decision cache emptied per
 * tenant by the Watch stream, read-your-writes after a resource write, and
 * offline verification of user access tokens.
 *
 * Every service declares the oldest Hayat it works with (minHayatVersion), and
 * create() refuses a Hayat older than that: each RPC in proto/hayat/authz/v1
 * says the release it first shipped in ("Since 1.2.0."), and the newest one
 * your service calls is its minimum. Hayat only adds to that API, so a newer
 * Hayat always works. checkHayatVersion() does the same check without
 * credentials, for a service that hasn't enrolled yet.
 */
import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-node';
import { createRemoteJWKSet, jwtVerify } from 'jose';
import { AuthzService, DirectoryService, SchemaService, ServerInfoService, TreeService, WatchService, } from './gen/hayat/authz/v1/authz_pb.js';
export * from './gen/hayat/authz/v1/authz_pb.js';
/**
 * The oldest Hayat any service can declare: the first release that reports its
 * version (ServerInfoService). A lower minHayatVersion is raised to it.
 */
export const HAYAT_VERSION_FLOOR = '1.2.0';
/** The Hayat a service reached is older than it declared it needs. `have` is '' when it's too old to say. */
export class HayatVersionError extends Error {
    have;
    need;
    apiUrl;
    constructor(have, need, apiUrl) {
        const running = have || `a release older than ${HAYAT_VERSION_FLOOR} (it doesn't report its version)`;
        super(`hayat: this service needs Hayat ${need} or newer, but ${apiUrl} runs ${running} — upgrade Hayat first`);
        this.have = have;
        this.need = need;
        this.apiUrl = apiUrl;
        this.name = 'HayatVersionError';
    }
}
const parseVersion = (s) => {
    const m = /^v?(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$/.exec(s);
    return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : null;
};
const compareVersions = (a, b) => a[0] - b[0] || a[1] - b[1] || a[2] - b[2];
/** A declared minimum, validated and raised to the floor. */
function minVersion(min) {
    if (!min)
        throw new Error(`hayat: minHayatVersion is required — the oldest Hayat release this service works with: the newest "Since" among the RPCs it calls (proto/hayat/authz/v1), e.g. '${HAYAT_VERSION_FLOOR}'`);
    const v = parseVersion(min);
    if (!v)
        throw new Error(`hayat: minHayatVersion '${min}' isn't a release like '${HAYAT_VERSION_FLOOR}'`);
    return compareVersions(v, parseVersion(HAYAT_VERSION_FLOOR)) < 0 ? HAYAT_VERSION_FLOOR : min.replace(/^v/, '');
}
/** have >= need. A version that isn't a release ('dev', a build from source) is new enough. */
export function hayatVersionAtLeast(have, need) {
    const h = parseVersion(have);
    const n = parseVersion(need);
    return !h || !n || compareVersions(h, n) >= 0;
}
/**
 * Ask the Hayat at apiUrl which release it is and refuse one older than min
 * (raised to HAYAT_VERSION_FLOOR). Needs no credentials, so a service calls it
 * before it enrolls; create() calls it on every start. → the version Hayat reported.
 */
export async function checkHayatVersion(apiUrl, min) {
    const need = minVersion(min);
    const baseUrl = apiUrl.replace(/\/$/, '');
    const info = createClient(ServerInfoService, createConnectTransport({ baseUrl, httpVersion: '1.1' }));
    let have;
    try {
        have = (await info.getServerInfo({})).version;
    }
    catch (err) {
        if (err instanceof ConnectError && (err.code === Code.Unimplemented || err.code === Code.NotFound))
            throw new HayatVersionError('', need, baseUrl);
        throw new Error(`hayat: couldn't read Hayat's version at ${baseUrl}: ${err instanceof Error ? err.message : err}`);
    }
    if (!hayatVersionAtLeast(have, need))
        throw new HayatVersionError(have, need, baseUrl);
    return have;
}
/** Names one of your resources, or a tenant with ref('hayat/tenant', tenantId). */
export const ref = (type, externalId) => ({ type, externalId });
const GLOBAL = '00000000-0000-0000-0000-000000000000';
export class Hayat {
    cfg;
    discovery;
    serverVersion;
    authz;
    tree;
    directory;
    schema;
    watcher;
    token = '';
    tokenExpires = 0;
    tokenPending = null;
    cache = new Map();
    lastWrite = { rev: 0n, at: 0 };
    jwks;
    constructor(cfg, discovery, 
    /** The release the Hayat reported when create() checked it ('dev' for a build from source). */
    serverVersion) {
        this.cfg = cfg;
        this.discovery = discovery;
        this.serverVersion = serverVersion;
        const auth = (next) => async (req) => {
            req.header.set('Authorization', `Bearer ${await this.accessToken()}`);
            return next(req);
        };
        const transport = createConnectTransport({ baseUrl: cfg.apiUrl.replace(/\/$/, ''), httpVersion: '1.1', interceptors: [auth] });
        this.authz = createClient(AuthzService, transport);
        this.tree = createClient(TreeService, transport);
        this.directory = createClient(DirectoryService, transport);
        this.schema = createClient(SchemaService, transport);
        this.watcher = createClient(WatchService, transport);
        this.jwks = createRemoteJWKSet(new URL(discovery.jwks_uri));
    }
    /** Check the Hayat's version (a HayatVersionError when it's too old), discover the issuer, and prepare the clients. */
    static async create(cfg) {
        const version = await checkHayatVersion(cfg.apiUrl, cfg.minHayatVersion);
        const res = await fetch(`${cfg.issuer.replace(/\/$/, '')}/.well-known/openid-configuration`);
        if (!res.ok)
            throw new Error(`hayat: discovery answered ${res.status}`);
        return new Hayat({ cacheTtlMs: 10_000, ...cfg }, (await res.json()), version);
    }
    /** A client credentials token, refreshed 30 s before it expires. */
    async accessToken() {
        if (this.token && Date.now() < this.tokenExpires - 30_000)
            return this.token;
        this.tokenPending ??= (async () => {
            try {
                const res = await fetch(this.discovery.token_endpoint, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/x-www-form-urlencoded',
                        Authorization: 'Basic ' + Buffer.from(`${encodeURIComponent(this.cfg.clientId)}:${encodeURIComponent(this.cfg.clientSecret)}`).toString('base64'),
                    },
                    body: new URLSearchParams({ grant_type: 'client_credentials' }),
                });
                if (!res.ok)
                    throw new Error(`hayat: token endpoint answered ${res.status}`);
                const body = (await res.json());
                this.token = body.access_token;
                this.tokenExpires = Date.now() + body.expires_in * 1000;
                return this.token;
            }
            finally {
                this.tokenPending = null;
            }
        })();
        return this.tokenPending;
    }
    consistency() {
        // For a moment after a write, ask for at least the revision it produced.
        if (Date.now() - this.lastWrite.at < 2000)
            return { $typeName: 'hayat.authz.v1.Consistency', atLeastRev: this.lastWrite.rev, fullyConsistent: false };
        return undefined;
    }
    refOf(r) {
        return { $typeName: 'hayat.authz.v1.ResourceRef', type: r.type ?? '', externalId: r.externalId ?? '', nodeId: r.nodeId ?? '' };
    }
    /** Whether a user holds a permission on a resource. An unknown resource is a no. */
    async check(userId, permission, resource) {
        const key = `${userId}\0${permission}\0${resource.type}\0${resource.externalId}\0${resource.nodeId}`;
        const hit = this.cache.get(key);
        if (hit && hit.expires > Date.now())
            return hit.allowed;
        try {
            const res = await this.authz.check({ userId, permission, resource: this.refOf(resource), consistency: this.consistency() });
            if (this.cfg.cacheTtlMs) {
                if (this.cache.size > 100_000)
                    this.cache.clear();
                this.cache.set(key, { allowed: res.allowed, tenant: res.tenantId, expires: Date.now() + this.cfg.cacheTtlMs });
            }
            return res.allowed;
        }
        catch (err) {
            if (err instanceof ConnectError && err.code === Code.NotFound)
                return false;
            throw err;
        }
    }
    /** Mirror one of your resources into the tree under parent. */
    async upsertResource(resource, name, parent, ownerUserId = '') {
        const res = await this.tree.upsertResource({
            type: resource.type ?? '',
            externalId: resource.externalId ?? '',
            name,
            parent: this.refOf(parent),
            ownerUserId,
        });
        this.wrote(res.tenantId, res.rev);
        return res.resource;
    }
    async deleteResource(resource) {
        const res = await this.tree.deleteResource({ type: resource.type ?? '', externalId: resource.externalId ?? '' });
        this.wrote(res.tenantId, res.rev);
    }
    wrote(tenant, rev) {
        this.lastWrite = { rev, at: Date.now() };
        this.dropTenant(tenant);
    }
    /** Ids of resources of one type where the user holds permission. */
    async lookup(userId, permission, type, tenantId = '') {
        const out = [];
        let pageToken = '';
        do {
            const res = await this.authz.lookupResources({ userId, permission, type, tenantId, pageSize: 500, pageToken });
            out.push(...res.resources.map((r) => r.externalId));
            pageToken = res.nextPageToken;
        } while (pageToken);
        return out;
    }
    /** Streams policy changes and drops cached decisions, reconnecting until the signal aborts. */
    async watch(signal) {
        let cursor = '';
        let backoff = 1000;
        while (!signal?.aborted) {
            try {
                for await (const res of this.watcher.watch({ cursor }, { signal })) {
                    const e = res.event;
                    if (!e)
                        continue;
                    cursor = e.id;
                    if (e.tenantId === GLOBAL)
                        this.cache.clear();
                    else
                        this.dropTenant(e.tenantId);
                    backoff = 1000;
                }
            }
            catch {
                /* reconnect below */
            }
            this.cache.clear(); // whatever happened while disconnected is unknown
            if (signal?.aborted)
                return;
            await new Promise((r) => setTimeout(r, backoff));
            backoff = Math.min(backoff * 2, 30_000);
        }
    }
    dropTenant(tenant) {
        for (const [k, v] of this.cache)
            if (v.tenant === tenant)
                this.cache.delete(k);
    }
    /**
     * Verify a user's access token offline: signature, issuer, expiry, and that
     * it was issued for `audience`. A sign-in token's `aud` names its client and
     * the client's service, so pass your service's namespace ("billing") to
     * accept any of its sign-in clients, or one client's id to accept only that
     * client. Leave it out to accept any.
     */
    async verifyAccessToken(token, audience) {
        const { payload } = await jwtVerify(token, this.jwks, { issuer: this.discovery.issuer });
        const claims = payload;
        if (claims.sub?.startsWith('client:'))
            throw new Error('hayat: a service token, not a person');
        if (audience) {
            const aud = Array.isArray(claims.aud) ? claims.aud : claims.aud ? [claims.aud] : [];
            if (claims.client_id !== audience && !aud.includes(audience))
                throw new Error('hayat: token was issued for another service or client');
        }
        return claims;
    }
    /** Express/Connect-style middleware: verifies the bearer token and sets req.hayatUser. */
    middleware(audience) {
        return async (req, res, next) => {
            const header = req.headers.authorization;
            const token = typeof header === 'string' && header.startsWith('Bearer ') ? header.slice(7) : '';
            try {
                if (!token)
                    throw new Error('no token');
                req.hayatUser = await this.verifyAccessToken(token, audience);
                next();
            }
            catch {
                res.statusCode = 401;
                res.end('sign in required');
            }
        };
    }
}
