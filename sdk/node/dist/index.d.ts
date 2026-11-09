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
import { type Client } from '@connectrpc/connect';
import { type JWTPayload } from 'jose';
import { AuthzService, DirectoryService, SchemaService, TreeService } from './gen/hayat/authz/v1/authz_pb.js';
export * from './gen/hayat/authz/v1/authz_pb.js';
export type HayatConfig = {
    /** The api role, e.g. http://hayat-api:7443. */
    apiUrl: string;
    /** The auth role's public URL; tokens and keys are discovered from it. */
    issuer: string;
    clientId: string;
    clientSecret: string;
    /** How long a decision is reused, in ms. Default 10 000; 0 turns the cache off. */
    cacheTtlMs?: number;
};
export type Resource = {
    type?: string;
    externalId?: string;
    nodeId?: string;
};
/** Names one of your resources, or a tenant with ref('hayat/tenant', tenantId). */
export declare const ref: (type: string, externalId: string) => Resource;
/** What a Hayat access token says about the person holding it. Never what they may do. */
export type UserClaims = JWTPayload & {
    sid?: string;
    client_id?: string;
};
export declare class Hayat {
    private readonly cfg;
    private readonly discovery;
    readonly authz: Client<typeof AuthzService>;
    readonly tree: Client<typeof TreeService>;
    readonly directory: Client<typeof DirectoryService>;
    readonly schema: Client<typeof SchemaService>;
    private readonly watcher;
    private token;
    private tokenExpires;
    private tokenPending;
    private cache;
    private lastWrite;
    private readonly jwks;
    private constructor();
    static create(cfg: HayatConfig): Promise<Hayat>;
    /** A client credentials token, refreshed 30 s before it expires. */
    private accessToken;
    private consistency;
    private refOf;
    /** Whether a user holds a permission on a resource. An unknown resource is a no. */
    check(userId: string, permission: string, resource: Resource): Promise<boolean>;
    /** Mirror one of your resources into the tree under parent. */
    upsertResource(resource: Resource, name: string, parent: Resource, ownerUserId?: string): Promise<import("./gen/hayat/authz/v1/authz_pb.js").Resource | undefined>;
    deleteResource(resource: Resource): Promise<void>;
    private wrote;
    /** Ids of resources of one type where the user holds permission. */
    lookup(userId: string, permission: string, type: string, tenantId?: string): Promise<string[]>;
    /** Streams policy changes and drops cached decisions, reconnecting until the signal aborts. */
    watch(signal?: AbortSignal): Promise<void>;
    private dropTenant;
    /**
     * Verify a user's access token offline: signature, issuer, expiry, and that
     * it was issued to clientId (your application's sign-in client).
     */
    verifyAccessToken(token: string, clientId?: string): Promise<UserClaims>;
    /** Express/Connect-style middleware: verifies the bearer token and sets req.hayatUser. */
    middleware(clientId?: string): (req: {
        headers: Record<string, string | string[] | undefined>;
        hayatUser?: UserClaims;
    }, res: {
        statusCode: number;
        end: (b?: string) => void;
    }, next: () => void) => Promise<void>;
}
