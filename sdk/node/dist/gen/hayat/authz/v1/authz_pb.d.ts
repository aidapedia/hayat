import type { GenFile, GenMessage, GenService } from "@bufbuild/protobuf/codegenv2";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import type { Message } from "@bufbuild/protobuf";
/**
 * Describes the file hayat/authz/v1/authz.proto.
 */
export declare const file_hayat_authz_v1_authz: GenFile;
/**
 * Names a node: a service resource by (type, external_id), a tenant by
 * ("hayat/tenant", tenant id), or any node by node_id.
 *
 * @generated from message hayat.authz.v1.ResourceRef
 */
export type ResourceRef = Message<"hayat.authz.v1.ResourceRef"> & {
    /**
     * @generated from field: string type = 1;
     */
    type: string;
    /**
     * @generated from field: string external_id = 2;
     */
    externalId: string;
    /**
     * @generated from field: string node_id = 3;
     */
    nodeId: string;
};
/**
 * Describes the message hayat.authz.v1.ResourceRef.
 * Use `create(ResourceRefSchema)` to create a new message.
 */
export declare const ResourceRefSchema: GenMessage<ResourceRef>;
/**
 * @generated from message hayat.authz.v1.Consistency
 */
export type Consistency = Message<"hayat.authz.v1.Consistency"> & {
    /**
     * Re-read the tenant's revision if the cached one is older: read your writes.
     *
     * @generated from field: int64 at_least_rev = 1;
     */
    atLeastRev: bigint;
    /**
     * Bypass every cache. For destructive actions.
     *
     * @generated from field: bool fully_consistent = 2;
     */
    fullyConsistent: boolean;
};
/**
 * Describes the message hayat.authz.v1.Consistency.
 * Use `create(ConsistencySchema)` to create a new message.
 */
export declare const ConsistencySchema: GenMessage<Consistency>;
/**
 * Why a permission is held: the first thing up the chain that gave it.
 *
 * @generated from message hayat.authz.v1.Reason
 */
export type Reason = Message<"hayat.authz.v1.Reason"> & {
    /**
     * owner | grant | placement | admin
     *
     * @generated from field: string kind = 1;
     */
    kind: string;
    /**
     * @generated from field: string node_id = 2;
     */
    nodeId: string;
    /**
     * @generated from field: string node_type = 3;
     */
    nodeType: string;
    /**
     * @generated from field: string node_name = 4;
     */
    nodeName: string;
    /**
     * @generated from field: string role = 5;
     */
    role: string;
    /**
     * @generated from field: string principal_type = 6;
     */
    principalType: string;
    /**
     * @generated from field: string principal_id = 7;
     */
    principalId: string;
    /**
     * The same, in a sentence.
     *
     * @generated from field: string text = 8;
     */
    text: string;
};
/**
 * Describes the message hayat.authz.v1.Reason.
 * Use `create(ReasonSchema)` to create a new message.
 */
export declare const ReasonSchema: GenMessage<Reason>;
/**
 * @generated from message hayat.authz.v1.Resource
 */
export type Resource = Message<"hayat.authz.v1.Resource"> & {
    /**
     * @generated from field: string node_id = 1;
     */
    nodeId: string;
    /**
     * @generated from field: string type = 2;
     */
    type: string;
    /**
     * @generated from field: string external_id = 3;
     */
    externalId: string;
    /**
     * @generated from field: string name = 4;
     */
    name: string;
    /**
     * @generated from field: string tenant_id = 5;
     */
    tenantId: string;
    /**
     * @generated from field: string parent_node_id = 6;
     */
    parentNodeId: string;
    /**
     * @generated from field: string owner_user_id = 7;
     */
    ownerUserId: string;
};
/**
 * Describes the message hayat.authz.v1.Resource.
 * Use `create(ResourceSchema)` to create a new message.
 */
export declare const ResourceSchema: GenMessage<Resource>;
/**
 * @generated from message hayat.authz.v1.CheckRequest
 */
export type CheckRequest = Message<"hayat.authz.v1.CheckRequest"> & {
    /**
     * @generated from field: string user_id = 1;
     */
    userId: string;
    /**
     * @generated from field: string permission = 2;
     */
    permission: string;
    /**
     * @generated from field: hayat.authz.v1.ResourceRef resource = 3;
     */
    resource?: ResourceRef | undefined;
    /**
     * @generated from field: hayat.authz.v1.Consistency consistency = 4;
     */
    consistency?: Consistency | undefined;
    /**
     * Return the reason. Off by default to keep answers small.
     *
     * @generated from field: bool explain = 5;
     */
    explain: boolean;
    /**
     * Ask about an agent instead of a person: set agent_id and leave user_id
     * empty. An agent holds less than a person could: see AgentService.
     *
     * @generated from field: string agent_id = 6;
     */
    agentId: string;
};
/**
 * Describes the message hayat.authz.v1.CheckRequest.
 * Use `create(CheckRequestSchema)` to create a new message.
 */
export declare const CheckRequestSchema: GenMessage<CheckRequest>;
/**
 * @generated from message hayat.authz.v1.CheckResponse
 */
export type CheckResponse = Message<"hayat.authz.v1.CheckResponse"> & {
    /**
     * @generated from field: bool allowed = 1;
     */
    allowed: boolean;
    /**
     * The tenant's revision the answer was computed at.
     *
     * @generated from field: int64 rev = 2;
     */
    rev: bigint;
    /**
     * @generated from field: string tenant_id = 3;
     */
    tenantId: string;
    /**
     * @generated from field: hayat.authz.v1.Reason reason = 4;
     */
    reason?: Reason | undefined;
};
/**
 * Describes the message hayat.authz.v1.CheckResponse.
 * Use `create(CheckResponseSchema)` to create a new message.
 */
export declare const CheckResponseSchema: GenMessage<CheckResponse>;
/**
 * @generated from message hayat.authz.v1.BatchCheckRequest
 */
export type BatchCheckRequest = Message<"hayat.authz.v1.BatchCheckRequest"> & {
    /**
     * @generated from field: repeated hayat.authz.v1.CheckRequest checks = 1;
     */
    checks: CheckRequest[];
};
/**
 * Describes the message hayat.authz.v1.BatchCheckRequest.
 * Use `create(BatchCheckRequestSchema)` to create a new message.
 */
export declare const BatchCheckRequestSchema: GenMessage<BatchCheckRequest>;
/**
 * @generated from message hayat.authz.v1.BatchCheckResponse
 */
export type BatchCheckResponse = Message<"hayat.authz.v1.BatchCheckResponse"> & {
    /**
     * In the order asked. A check that couldn't be answered (unknown resource)
     * is allowed = false with an empty rev.
     *
     * @generated from field: repeated hayat.authz.v1.CheckResponse results = 1;
     */
    results: CheckResponse[];
};
/**
 * Describes the message hayat.authz.v1.BatchCheckResponse.
 * Use `create(BatchCheckResponseSchema)` to create a new message.
 */
export declare const BatchCheckResponseSchema: GenMessage<BatchCheckResponse>;
/**
 * @generated from message hayat.authz.v1.PermissionsAtRequest
 */
export type PermissionsAtRequest = Message<"hayat.authz.v1.PermissionsAtRequest"> & {
    /**
     * @generated from field: string user_id = 1;
     */
    userId: string;
    /**
     * @generated from field: hayat.authz.v1.ResourceRef resource = 2;
     */
    resource?: ResourceRef | undefined;
    /**
     * @generated from field: hayat.authz.v1.Consistency consistency = 3;
     */
    consistency?: Consistency | undefined;
    /**
     * An agent instead of a person, as in CheckRequest.
     *
     * @generated from field: string agent_id = 4;
     */
    agentId: string;
};
/**
 * Describes the message hayat.authz.v1.PermissionsAtRequest.
 * Use `create(PermissionsAtRequestSchema)` to create a new message.
 */
export declare const PermissionsAtRequestSchema: GenMessage<PermissionsAtRequest>;
/**
 * @generated from message hayat.authz.v1.PermissionsAtResponse
 */
export type PermissionsAtResponse = Message<"hayat.authz.v1.PermissionsAtResponse"> & {
    /**
     * @generated from field: repeated string permissions = 1;
     */
    permissions: string[];
    /**
     * @generated from field: int64 rev = 2;
     */
    rev: bigint;
    /**
     * @generated from field: string tenant_id = 3;
     */
    tenantId: string;
};
/**
 * Describes the message hayat.authz.v1.PermissionsAtResponse.
 * Use `create(PermissionsAtResponseSchema)` to create a new message.
 */
export declare const PermissionsAtResponseSchema: GenMessage<PermissionsAtResponse>;
/**
 * @generated from message hayat.authz.v1.LookupResourcesRequest
 */
export type LookupResourcesRequest = Message<"hayat.authz.v1.LookupResourcesRequest"> & {
    /**
     * @generated from field: string user_id = 1;
     */
    userId: string;
    /**
     * @generated from field: string permission = 2;
     */
    permission: string;
    /**
     * @generated from field: string type = 3;
     */
    type: string;
    /**
     * Optional: only this tenant.
     *
     * @generated from field: string tenant_id = 4;
     */
    tenantId: string;
    /**
     * @generated from field: int32 page_size = 5;
     */
    pageSize: number;
    /**
     * @generated from field: string page_token = 6;
     */
    pageToken: string;
    /**
     * @generated from field: hayat.authz.v1.Consistency consistency = 7;
     */
    consistency?: Consistency | undefined;
    /**
     * An agent instead of a person, as in CheckRequest.
     *
     * @generated from field: string agent_id = 8;
     */
    agentId: string;
};
/**
 * Describes the message hayat.authz.v1.LookupResourcesRequest.
 * Use `create(LookupResourcesRequestSchema)` to create a new message.
 */
export declare const LookupResourcesRequestSchema: GenMessage<LookupResourcesRequest>;
/**
 * @generated from message hayat.authz.v1.LookupResourcesResponse
 */
export type LookupResourcesResponse = Message<"hayat.authz.v1.LookupResourcesResponse"> & {
    /**
     * @generated from field: repeated hayat.authz.v1.Resource resources = 1;
     */
    resources: Resource[];
    /**
     * @generated from field: string next_page_token = 2;
     */
    nextPageToken: string;
};
/**
 * Describes the message hayat.authz.v1.LookupResourcesResponse.
 * Use `create(LookupResourcesResponseSchema)` to create a new message.
 */
export declare const LookupResourcesResponseSchema: GenMessage<LookupResourcesResponse>;
/**
 * @generated from message hayat.authz.v1.LookupSubjectsRequest
 */
export type LookupSubjectsRequest = Message<"hayat.authz.v1.LookupSubjectsRequest"> & {
    /**
     * @generated from field: string permission = 1;
     */
    permission: string;
    /**
     * @generated from field: hayat.authz.v1.ResourceRef resource = 2;
     */
    resource?: ResourceRef | undefined;
    /**
     * @generated from field: hayat.authz.v1.Consistency consistency = 3;
     */
    consistency?: Consistency | undefined;
};
/**
 * Describes the message hayat.authz.v1.LookupSubjectsRequest.
 * Use `create(LookupSubjectsRequestSchema)` to create a new message.
 */
export declare const LookupSubjectsRequestSchema: GenMessage<LookupSubjectsRequest>;
/**
 * @generated from message hayat.authz.v1.Subject
 */
export type Subject = Message<"hayat.authz.v1.Subject"> & {
    /**
     * A person's id; empty when the subject is an agent.
     *
     * @generated from field: string user_id = 1;
     */
    userId: string;
    /**
     * @generated from field: hayat.authz.v1.Reason reason = 2;
     */
    reason?: Reason | undefined;
    /**
     * An agent's id; empty when the subject is a person.
     *
     * @generated from field: string agent_id = 3;
     */
    agentId: string;
};
/**
 * Describes the message hayat.authz.v1.Subject.
 * Use `create(SubjectSchema)` to create a new message.
 */
export declare const SubjectSchema: GenMessage<Subject>;
/**
 * @generated from message hayat.authz.v1.LookupSubjectsResponse
 */
export type LookupSubjectsResponse = Message<"hayat.authz.v1.LookupSubjectsResponse"> & {
    /**
     * @generated from field: repeated hayat.authz.v1.Subject subjects = 1;
     */
    subjects: Subject[];
};
/**
 * Describes the message hayat.authz.v1.LookupSubjectsResponse.
 * Use `create(LookupSubjectsResponseSchema)` to create a new message.
 */
export declare const LookupSubjectsResponseSchema: GenMessage<LookupSubjectsResponse>;
/**
 * @generated from message hayat.authz.v1.UpsertResourceRequest
 */
export type UpsertResourceRequest = Message<"hayat.authz.v1.UpsertResourceRequest"> & {
    /**
     * @generated from field: string type = 1;
     */
    type: string;
    /**
     * @generated from field: string external_id = 2;
     */
    externalId: string;
    /**
     * @generated from field: string name = 3;
     */
    name: string;
    /**
     * @generated from field: hayat.authz.v1.ResourceRef parent = 4;
     */
    parent?: ResourceRef | undefined;
    /**
     * Optional. The owner can do everything at and under the node.
     *
     * @generated from field: string owner_user_id = 5;
     */
    ownerUserId: string;
};
/**
 * Describes the message hayat.authz.v1.UpsertResourceRequest.
 * Use `create(UpsertResourceRequestSchema)` to create a new message.
 */
export declare const UpsertResourceRequestSchema: GenMessage<UpsertResourceRequest>;
/**
 * @generated from message hayat.authz.v1.DeleteResourceRequest
 */
export type DeleteResourceRequest = Message<"hayat.authz.v1.DeleteResourceRequest"> & {
    /**
     * @generated from field: string type = 1;
     */
    type: string;
    /**
     * @generated from field: string external_id = 2;
     */
    externalId: string;
};
/**
 * Describes the message hayat.authz.v1.DeleteResourceRequest.
 * Use `create(DeleteResourceRequestSchema)` to create a new message.
 */
export declare const DeleteResourceRequestSchema: GenMessage<DeleteResourceRequest>;
/**
 * @generated from message hayat.authz.v1.GetResourceRequest
 */
export type GetResourceRequest = Message<"hayat.authz.v1.GetResourceRequest"> & {
    /**
     * @generated from field: hayat.authz.v1.ResourceRef resource = 1;
     */
    resource?: ResourceRef | undefined;
};
/**
 * Describes the message hayat.authz.v1.GetResourceRequest.
 * Use `create(GetResourceRequestSchema)` to create a new message.
 */
export declare const GetResourceRequestSchema: GenMessage<GetResourceRequest>;
/**
 * @generated from message hayat.authz.v1.UpsertResourceResponse
 */
export type UpsertResourceResponse = Message<"hayat.authz.v1.UpsertResourceResponse"> & {
    /**
     * Send this as at_least_rev on the next check to see the change.
     *
     * @generated from field: int64 rev = 1;
     */
    rev: bigint;
    /**
     * @generated from field: string tenant_id = 2;
     */
    tenantId: string;
    /**
     * @generated from field: hayat.authz.v1.Resource resource = 3;
     */
    resource?: Resource | undefined;
};
/**
 * Describes the message hayat.authz.v1.UpsertResourceResponse.
 * Use `create(UpsertResourceResponseSchema)` to create a new message.
 */
export declare const UpsertResourceResponseSchema: GenMessage<UpsertResourceResponse>;
/**
 * @generated from message hayat.authz.v1.DeleteResourceResponse
 */
export type DeleteResourceResponse = Message<"hayat.authz.v1.DeleteResourceResponse"> & {
    /**
     * @generated from field: int64 rev = 1;
     */
    rev: bigint;
    /**
     * @generated from field: string tenant_id = 2;
     */
    tenantId: string;
};
/**
 * Describes the message hayat.authz.v1.DeleteResourceResponse.
 * Use `create(DeleteResourceResponseSchema)` to create a new message.
 */
export declare const DeleteResourceResponseSchema: GenMessage<DeleteResourceResponse>;
/**
 * @generated from message hayat.authz.v1.GetResourceResponse
 */
export type GetResourceResponse = Message<"hayat.authz.v1.GetResourceResponse"> & {
    /**
     * @generated from field: hayat.authz.v1.Resource resource = 1;
     */
    resource?: Resource | undefined;
};
/**
 * Describes the message hayat.authz.v1.GetResourceResponse.
 * Use `create(GetResourceResponseSchema)` to create a new message.
 */
export declare const GetResourceResponseSchema: GenMessage<GetResourceResponse>;
/**
 * A person's theme. Every field is optional: an empty one means "your app's
 * own default", so it only carries what the person changed. Colors are #rrggbb.
 *
 * @generated from message hayat.authz.v1.Appearance
 */
export type Appearance = Message<"hayat.authz.v1.Appearance"> & {
    /**
     * The preset it started from ("tg-dark", "hayat-light", …) or "custom".
     *
     * @generated from field: string preset = 1;
     */
    preset: string;
    /**
     * system | light | dark: the neutral palette behind the colors left empty.
     *
     * @generated from field: string mode = 2;
     */
    mode: string;
    /**
     * @generated from field: string accent = 3;
     */
    accent: string;
    /**
     * @generated from field: string background = 4;
     */
    background: string;
    /**
     * @generated from field: string surface = 5;
     */
    surface: string;
    /**
     * @generated from field: string text = 6;
     */
    text: string;
    /**
     * @generated from field: string muted = 7;
     */
    muted: string;
    /**
     * @generated from field: string border = 8;
     */
    border: string;
    /**
     * The sidebar follows the content colors; the four below are ignored.
     *
     * @generated from field: bool sidebar_match_content = 9;
     */
    sidebarMatchContent: boolean;
    /**
     * @generated from field: string sidebar_background = 10;
     */
    sidebarBackground: string;
    /**
     * @generated from field: string sidebar_text = 11;
     */
    sidebarText: string;
    /**
     * @generated from field: string sidebar_active = 12;
     */
    sidebarActive: string;
    /**
     * @generated from field: string sidebar_border = 13;
     */
    sidebarBorder: string;
    /**
     * @generated from field: string success = 14;
     */
    success: string;
    /**
     * @generated from field: string warning = 15;
     */
    warning: string;
    /**
     * @generated from field: string destructive = 16;
     */
    destructive: string;
    /**
     * system | inter | plex
     *
     * @generated from field: string font = 17;
     */
    font: string;
    /**
     * The root font size in pixels, 12–18.
     *
     * @generated from field: optional int32 text_size = 18;
     */
    textSize?: number | undefined;
    /**
     * comfortable | compact | condensed
     *
     * @generated from field: string density = 19;
     */
    density: string;
    /**
     * Corner radius in pixels, 0–18.
     *
     * @generated from field: optional int32 radius = 20;
     */
    radius?: number | undefined;
    /**
     * Sidebar width in pixels, 200–320.
     *
     * @generated from field: optional int32 sidebar_width = 21;
     */
    sidebarWidth?: number | undefined;
    /**
     * narrow | default | full
     *
     * @generated from field: string content_width = 22;
     */
    contentWidth: string;
    /**
     * Text on the accent: buttons and anything else filled with it. Empty is
     * black or white, whichever reads better on the accent.
     *
     * @generated from field: string accent_text = 23;
     */
    accentText: string;
    /**
     * The interface's size as a percent of the app's own text size, 80–120.
     * It wins over text_size, which only older themes set.
     *
     * @generated from field: optional int32 scale = 24;
     */
    scale?: number | undefined;
};
/**
 * Describes the message hayat.authz.v1.Appearance.
 * Use `create(AppearanceSchema)` to create a new message.
 */
export declare const AppearanceSchema: GenMessage<Appearance>;
/**
 * @generated from message hayat.authz.v1.GetUserAppearanceRequest
 */
export type GetUserAppearanceRequest = Message<"hayat.authz.v1.GetUserAppearanceRequest"> & {
    /**
     * @generated from field: string user_id = 1;
     */
    userId: string;
};
/**
 * Describes the message hayat.authz.v1.GetUserAppearanceRequest.
 * Use `create(GetUserAppearanceRequestSchema)` to create a new message.
 */
export declare const GetUserAppearanceRequestSchema: GenMessage<GetUserAppearanceRequest>;
/**
 * @generated from message hayat.authz.v1.GetUserAppearanceResponse
 */
export type GetUserAppearanceResponse = Message<"hayat.authz.v1.GetUserAppearanceResponse"> & {
    /**
     * Absent when the person never chose a theme.
     *
     * @generated from field: hayat.authz.v1.Appearance appearance = 1;
     */
    appearance?: Appearance | undefined;
    /**
     * The person gave the calling service a theme of its own; otherwise this is
     * their account's theme.
     *
     * @generated from field: bool own = 2;
     */
    own: boolean;
};
/**
 * Describes the message hayat.authz.v1.GetUserAppearanceResponse.
 * Use `create(GetUserAppearanceResponseSchema)` to create a new message.
 */
export declare const GetUserAppearanceResponseSchema: GenMessage<GetUserAppearanceResponse>;
/**
 * @generated from message hayat.authz.v1.Tenant
 */
export type Tenant = Message<"hayat.authz.v1.Tenant"> & {
    /**
     * @generated from field: string id = 1;
     */
    id: string;
    /**
     * @generated from field: string slug = 2;
     */
    slug: string;
    /**
     * @generated from field: string name = 3;
     */
    name: string;
    /**
     * @generated from field: string owner_user_id = 4;
     */
    ownerUserId: string;
    /**
     * The tenant's node in the tree: a parent for UpsertResource.
     *
     * @generated from field: string node_id = 5;
     */
    nodeId: string;
    /**
     * @generated from field: int32 member_count = 6;
     */
    memberCount: number;
    /**
     * <issuer>/media/<id>, empty for none.
     *
     * @generated from field: string logo_url = 7;
     */
    logoUrl: string;
};
/**
 * Describes the message hayat.authz.v1.Tenant.
 * Use `create(TenantSchema)` to create a new message.
 */
export declare const TenantSchema: GenMessage<Tenant>;
/**
 * @generated from message hayat.authz.v1.ListUserTenantsRequest
 */
export type ListUserTenantsRequest = Message<"hayat.authz.v1.ListUserTenantsRequest"> & {
    /**
     * @generated from field: string user_id = 1;
     */
    userId: string;
};
/**
 * Describes the message hayat.authz.v1.ListUserTenantsRequest.
 * Use `create(ListUserTenantsRequestSchema)` to create a new message.
 */
export declare const ListUserTenantsRequestSchema: GenMessage<ListUserTenantsRequest>;
/**
 * @generated from message hayat.authz.v1.ListUserTenantsResponse
 */
export type ListUserTenantsResponse = Message<"hayat.authz.v1.ListUserTenantsResponse"> & {
    /**
     * @generated from field: repeated hayat.authz.v1.Tenant tenants = 1;
     */
    tenants: Tenant[];
};
/**
 * Describes the message hayat.authz.v1.ListUserTenantsResponse.
 * Use `create(ListUserTenantsResponseSchema)` to create a new message.
 */
export declare const ListUserTenantsResponseSchema: GenMessage<ListUserTenantsResponse>;
/**
 * @generated from message hayat.authz.v1.ListTenantMembersRequest
 */
export type ListTenantMembersRequest = Message<"hayat.authz.v1.ListTenantMembersRequest"> & {
    /**
     * @generated from field: string tenant_id = 1;
     */
    tenantId: string;
};
/**
 * Describes the message hayat.authz.v1.ListTenantMembersRequest.
 * Use `create(ListTenantMembersRequestSchema)` to create a new message.
 */
export declare const ListTenantMembersRequestSchema: GenMessage<ListTenantMembersRequest>;
/**
 * @generated from message hayat.authz.v1.TenantMember
 */
export type TenantMember = Message<"hayat.authz.v1.TenantMember"> & {
    /**
     * @generated from field: hayat.authz.v1.User user = 1;
     */
    user?: User | undefined;
    /**
     * @generated from field: bool is_owner = 2;
     */
    isOwner: boolean;
    /**
     * @generated from field: google.protobuf.Timestamp joined_at = 3;
     */
    joinedAt?: Timestamp | undefined;
};
/**
 * Describes the message hayat.authz.v1.TenantMember.
 * Use `create(TenantMemberSchema)` to create a new message.
 */
export declare const TenantMemberSchema: GenMessage<TenantMember>;
/**
 * @generated from message hayat.authz.v1.ListTenantMembersResponse
 */
export type ListTenantMembersResponse = Message<"hayat.authz.v1.ListTenantMembersResponse"> & {
    /**
     * @generated from field: repeated hayat.authz.v1.TenantMember members = 1;
     */
    members: TenantMember[];
};
/**
 * Describes the message hayat.authz.v1.ListTenantMembersResponse.
 * Use `create(ListTenantMembersResponseSchema)` to create a new message.
 */
export declare const ListTenantMembersResponseSchema: GenMessage<ListTenantMembersResponse>;
/**
 * @generated from message hayat.authz.v1.ListTenantGrantsRequest
 */
export type ListTenantGrantsRequest = Message<"hayat.authz.v1.ListTenantGrantsRequest"> & {
    /**
     * @generated from field: string tenant_id = 1;
     */
    tenantId: string;
    /**
     * Only these members; empty for everyone on the roster.
     *
     * @generated from field: repeated string user_ids = 2;
     */
    userIds: string[];
};
/**
 * Describes the message hayat.authz.v1.ListTenantGrantsRequest.
 * Use `create(ListTenantGrantsRequestSchema)` to create a new message.
 */
export declare const ListTenantGrantsRequestSchema: GenMessage<ListTenantGrantsRequest>;
/**
 * @generated from message hayat.authz.v1.MemberGrant
 */
export type MemberGrant = Message<"hayat.authz.v1.MemberGrant"> & {
    /**
     * @generated from field: string user_id = 1;
     */
    userId: string;
    /**
     * Where it was granted, or the node they own.
     *
     * @generated from field: hayat.authz.v1.Resource resource = 2;
     */
    resource?: Resource | undefined;
    /**
     * How it reaches the member: "owner" (they own the node), "user" (granted to
     * them), "group" (to a group they are on) or "everyone" (to the tenant).
     *
     * @generated from field: string via = 3;
     */
    via: string;
    /**
     * The role's slug and display name; empty when via is "owner".
     *
     * @generated from field: string role = 4;
     */
    role: string;
    /**
     * @generated from field: string role_name = 5;
     */
    roleName: string;
    /**
     * Reaches the nodes below as well.
     *
     * @generated from field: bool inherit = 6;
     */
    inherit: boolean;
    /**
     * The group it was granted to, when via is "group".
     *
     * @generated from field: string group_id = 7;
     */
    groupId: string;
    /**
     * @generated from field: string group_name = 8;
     */
    groupName: string;
};
/**
 * Describes the message hayat.authz.v1.MemberGrant.
 * Use `create(MemberGrantSchema)` to create a new message.
 */
export declare const MemberGrantSchema: GenMessage<MemberGrant>;
/**
 * @generated from message hayat.authz.v1.ListTenantGrantsResponse
 */
export type ListTenantGrantsResponse = Message<"hayat.authz.v1.ListTenantGrantsResponse"> & {
    /**
     * @generated from field: repeated hayat.authz.v1.MemberGrant grants = 1;
     */
    grants: MemberGrant[];
};
/**
 * Describes the message hayat.authz.v1.ListTenantGrantsResponse.
 * Use `create(ListTenantGrantsResponseSchema)` to create a new message.
 */
export declare const ListTenantGrantsResponseSchema: GenMessage<ListTenantGrantsResponse>;
/**
 * @generated from message hayat.authz.v1.User
 */
export type User = Message<"hayat.authz.v1.User"> & {
    /**
     * @generated from field: string id = 1;
     */
    id: string;
    /**
     * @generated from field: string email = 2;
     */
    email: string;
    /**
     * @generated from field: bool email_verified = 3;
     */
    emailVerified: boolean;
    /**
     * @generated from field: string name = 4;
     */
    name: string;
    /**
     * @generated from field: string picture = 5;
     */
    picture: string;
    /**
     * @generated from field: string locale = 6;
     */
    locale: string;
    /**
     * @generated from field: string zoneinfo = 7;
     */
    zoneinfo: string;
    /**
     * pending | active | disabled
     *
     * @generated from field: string status = 8;
     */
    status: string;
};
/**
 * Describes the message hayat.authz.v1.User.
 * Use `create(UserSchema)` to create a new message.
 */
export declare const UserSchema: GenMessage<User>;
/**
 * @generated from message hayat.authz.v1.GetUserRequest
 */
export type GetUserRequest = Message<"hayat.authz.v1.GetUserRequest"> & {
    /**
     * @generated from field: string id = 1;
     */
    id: string;
};
/**
 * Describes the message hayat.authz.v1.GetUserRequest.
 * Use `create(GetUserRequestSchema)` to create a new message.
 */
export declare const GetUserRequestSchema: GenMessage<GetUserRequest>;
/**
 * @generated from message hayat.authz.v1.GetUserResponse
 */
export type GetUserResponse = Message<"hayat.authz.v1.GetUserResponse"> & {
    /**
     * @generated from field: hayat.authz.v1.User user = 1;
     */
    user?: User | undefined;
};
/**
 * Describes the message hayat.authz.v1.GetUserResponse.
 * Use `create(GetUserResponseSchema)` to create a new message.
 */
export declare const GetUserResponseSchema: GenMessage<GetUserResponse>;
/**
 * @generated from message hayat.authz.v1.BatchGetUsersRequest
 */
export type BatchGetUsersRequest = Message<"hayat.authz.v1.BatchGetUsersRequest"> & {
    /**
     * @generated from field: repeated string ids = 1;
     */
    ids: string[];
};
/**
 * Describes the message hayat.authz.v1.BatchGetUsersRequest.
 * Use `create(BatchGetUsersRequestSchema)` to create a new message.
 */
export declare const BatchGetUsersRequestSchema: GenMessage<BatchGetUsersRequest>;
/**
 * @generated from message hayat.authz.v1.BatchGetUsersResponse
 */
export type BatchGetUsersResponse = Message<"hayat.authz.v1.BatchGetUsersResponse"> & {
    /**
     * Unknown ids are left out.
     *
     * @generated from field: repeated hayat.authz.v1.User users = 1;
     */
    users: User[];
};
/**
 * Describes the message hayat.authz.v1.BatchGetUsersResponse.
 * Use `create(BatchGetUsersResponseSchema)` to create a new message.
 */
export declare const BatchGetUsersResponseSchema: GenMessage<BatchGetUsersResponse>;
/**
 * @generated from message hayat.authz.v1.WatchRequest
 */
export type WatchRequest = Message<"hayat.authz.v1.WatchRequest"> & {
    /**
     * The id of the last event you processed; empty starts from now.
     *
     * @generated from field: string cursor = 1;
     */
    cursor: string;
    /**
     * Ask for a heartbeat: a response with no event as soon as the stream opens
     * and after every 30 seconds without one, so a proxy in between (Cloudflare
     * ends a request that's quiet for 100 seconds) keeps the stream open. Skip a
     * response without an event. Since 1.3.0; an older Hayat ignores it.
     *
     * @generated from field: bool heartbeat = 2;
     */
    heartbeat: boolean;
};
/**
 * Describes the message hayat.authz.v1.WatchRequest.
 * Use `create(WatchRequestSchema)` to create a new message.
 */
export declare const WatchRequestSchema: GenMessage<WatchRequest>;
/**
 * @generated from message hayat.authz.v1.WatchResponse
 */
export type WatchResponse = Message<"hayat.authz.v1.WatchResponse"> & {
    /**
     * Empty on a heartbeat.
     *
     * @generated from field: hayat.authz.v1.PolicyEvent event = 1;
     */
    event?: PolicyEvent | undefined;
};
/**
 * Describes the message hayat.authz.v1.WatchResponse.
 * Use `create(WatchResponseSchema)` to create a new message.
 */
export declare const WatchResponseSchema: GenMessage<WatchResponse>;
/**
 * @generated from message hayat.authz.v1.PolicyEvent
 */
export type PolicyEvent = Message<"hayat.authz.v1.PolicyEvent"> & {
    /**
     * @generated from field: string id = 1;
     */
    id: string;
    /**
     * 00000000-0000-0000-0000-000000000000 for a global change.
     *
     * @generated from field: string tenant_id = 2;
     */
    tenantId: string;
    /**
     * @generated from field: int64 rev = 3;
     */
    rev: bigint;
    /**
     * @generated from field: string kind = 4;
     */
    kind: string;
    /**
     * @generated from field: google.protobuf.Timestamp at = 5;
     */
    at?: Timestamp | undefined;
};
/**
 * Describes the message hayat.authz.v1.PolicyEvent.
 * Use `create(PolicyEventSchema)` to create a new message.
 */
export declare const PolicyEventSchema: GenMessage<PolicyEvent>;
/**
 * @generated from message hayat.authz.v1.Agent
 */
export type Agent = Message<"hayat.authz.v1.Agent"> & {
    /**
     * @generated from field: string id = 1;
     */
    id: string;
    /**
     * @generated from field: string tenant_id = 2;
     */
    tenantId: string;
    /**
     * @generated from field: string handle = 3;
     */
    handle: string;
    /**
     * @generated from field: string name = 4;
     */
    name: string;
    /**
     * @generated from field: string sponsor_user_id = 5;
     */
    sponsorUserId: string;
    /**
     * active | paused | disabled
     *
     * @generated from field: string status = 6;
     */
    status: string;
    /**
     * Why it isn't active: "sponsor_left", or empty when a person paused it.
     *
     * @generated from field: string status_reason = 7;
     */
    statusReason: string;
    /**
     * The services (namespaces) it works in.
     *
     * @generated from field: repeated string services = 8;
     */
    services: string[];
    /**
     * @generated from field: repeated string group_ids = 9;
     */
    groupIds: string[];
};
/**
 * Describes the message hayat.authz.v1.Agent.
 * Use `create(AgentSchema)` to create a new message.
 */
export declare const AgentSchema: GenMessage<Agent>;
/**
 * @generated from message hayat.authz.v1.GetAgentRequest
 */
export type GetAgentRequest = Message<"hayat.authz.v1.GetAgentRequest"> & {
    /**
     * @generated from field: string agent_id = 1;
     */
    agentId: string;
};
/**
 * Describes the message hayat.authz.v1.GetAgentRequest.
 * Use `create(GetAgentRequestSchema)` to create a new message.
 */
export declare const GetAgentRequestSchema: GenMessage<GetAgentRequest>;
/**
 * @generated from message hayat.authz.v1.GetAgentResponse
 */
export type GetAgentResponse = Message<"hayat.authz.v1.GetAgentResponse"> & {
    /**
     * @generated from field: hayat.authz.v1.Agent agent = 1;
     */
    agent?: Agent | undefined;
};
/**
 * Describes the message hayat.authz.v1.GetAgentResponse.
 * Use `create(GetAgentResponseSchema)` to create a new message.
 */
export declare const GetAgentResponseSchema: GenMessage<GetAgentResponse>;
/**
 * @generated from message hayat.authz.v1.ListTenantAgentsRequest
 */
export type ListTenantAgentsRequest = Message<"hayat.authz.v1.ListTenantAgentsRequest"> & {
    /**
     * @generated from field: string tenant_id = 1;
     */
    tenantId: string;
};
/**
 * Describes the message hayat.authz.v1.ListTenantAgentsRequest.
 * Use `create(ListTenantAgentsRequestSchema)` to create a new message.
 */
export declare const ListTenantAgentsRequestSchema: GenMessage<ListTenantAgentsRequest>;
/**
 * @generated from message hayat.authz.v1.ListTenantAgentsResponse
 */
export type ListTenantAgentsResponse = Message<"hayat.authz.v1.ListTenantAgentsResponse"> & {
    /**
     * @generated from field: repeated hayat.authz.v1.Agent agents = 1;
     */
    agents: Agent[];
};
/**
 * Describes the message hayat.authz.v1.ListTenantAgentsResponse.
 * Use `create(ListTenantAgentsResponseSchema)` to create a new message.
 */
export declare const ListTenantAgentsResponseSchema: GenMessage<ListTenantAgentsResponse>;
/**
 * @generated from message hayat.authz.v1.IssueAgentTokenRequest
 */
export type IssueAgentTokenRequest = Message<"hayat.authz.v1.IssueAgentTokenRequest"> & {
    /**
     * @generated from field: string agent_id = 1;
     */
    agentId: string;
};
/**
 * Describes the message hayat.authz.v1.IssueAgentTokenRequest.
 * Use `create(IssueAgentTokenRequestSchema)` to create a new message.
 */
export declare const IssueAgentTokenRequestSchema: GenMessage<IssueAgentTokenRequest>;
/**
 * @generated from message hayat.authz.v1.IssueAgentTokenResponse
 */
export type IssueAgentTokenResponse = Message<"hayat.authz.v1.IssueAgentTokenResponse"> & {
    /**
     * @generated from field: string access_token = 1;
     */
    accessToken: string;
    /**
     * @generated from field: google.protobuf.Timestamp expires_at = 2;
     */
    expiresAt?: Timestamp | undefined;
};
/**
 * Describes the message hayat.authz.v1.IssueAgentTokenResponse.
 * Use `create(IssueAgentTokenResponseSchema)` to create a new message.
 */
export declare const IssueAgentTokenResponseSchema: GenMessage<IssueAgentTokenResponse>;
/**
 * @generated from message hayat.authz.v1.PublishContractRequest
 */
export type PublishContractRequest = Message<"hayat.authz.v1.PublishContractRequest"> & {
    /**
     * YAML or JSON.
     *
     * @generated from field: string manifest = 1;
     */
    manifest: string;
};
/**
 * Describes the message hayat.authz.v1.PublishContractRequest.
 * Use `create(PublishContractRequestSchema)` to create a new message.
 */
export declare const PublishContractRequestSchema: GenMessage<PublishContractRequest>;
/**
 * @generated from message hayat.authz.v1.PublishContractResponse
 */
export type PublishContractResponse = Message<"hayat.authz.v1.PublishContractResponse"> & {
    /**
     * @generated from field: int32 version = 1;
     */
    version: number;
    /**
     * @generated from field: string status = 2;
     */
    status: string;
    /**
     * @generated from field: repeated string namespaces = 3;
     */
    namespaces: string[];
};
/**
 * Describes the message hayat.authz.v1.PublishContractResponse.
 * Use `create(PublishContractResponseSchema)` to create a new message.
 */
export declare const PublishContractResponseSchema: GenMessage<PublishContractResponse>;
/**
 * @generated from message hayat.authz.v1.InspectEnrollmentRequest
 */
export type InspectEnrollmentRequest = Message<"hayat.authz.v1.InspectEnrollmentRequest"> & {
    /**
     * @generated from field: string code = 1;
     */
    code: string;
};
/**
 * Describes the message hayat.authz.v1.InspectEnrollmentRequest.
 * Use `create(InspectEnrollmentRequestSchema)` to create a new message.
 */
export declare const InspectEnrollmentRequestSchema: GenMessage<InspectEnrollmentRequest>;
/**
 * @generated from message hayat.authz.v1.InspectEnrollmentResponse
 */
export type InspectEnrollmentResponse = Message<"hayat.authz.v1.InspectEnrollmentResponse"> & {
    /**
     * @generated from field: string service_id = 1;
     */
    serviceId: string;
    /**
     * The name it registers under (or the service's name when it exists).
     *
     * @generated from field: string display_name = 2;
     */
    displayName: string;
    /**
     * Redeeming it registers the service; otherwise it rotates its API secret.
     *
     * @generated from field: bool registers = 3;
     */
    registers: boolean;
    /**
     * Who made the code: by default, the admin of a service it registers.
     *
     * @generated from field: string made_by_email = 4;
     */
    madeByEmail: string;
    /**
     * @generated from field: string made_by_name = 5;
     */
    madeByName: string;
    /**
     * @generated from field: google.protobuf.Timestamp expires_at = 6;
     */
    expiresAt?: Timestamp | undefined;
    /**
     * @generated from field: string issuer = 7;
     */
    issuer: string;
};
/**
 * Describes the message hayat.authz.v1.InspectEnrollmentResponse.
 * Use `create(InspectEnrollmentResponseSchema)` to create a new message.
 */
export declare const InspectEnrollmentResponseSchema: GenMessage<InspectEnrollmentResponse>;
/**
 * @generated from message hayat.authz.v1.EnrollRequest
 */
export type EnrollRequest = Message<"hayat.authz.v1.EnrollRequest"> & {
    /**
     * "hye_…", from the console or `hayat enrollment-code`.
     *
     * @generated from field: string code = 1;
     */
    code: string;
    /**
     * The service's contract (YAML or JSON), published when it is newer than
     * the latest version. Optional.
     *
     * @generated from field: string manifest = 2;
     */
    manifest: string;
    /**
     * Used when this registers the service.
     *
     * @generated from field: string display_name = 3;
     */
    displayName: string;
    /**
     * @generated from field: string description = 4;
     */
    description: string;
    /**
     * The service's public address. When set, a public sign-in client with
     * <app_url>/callback as a redirect is made, or kept if one exists.
     *
     * @generated from field: string app_url = 5;
     */
    appUrl: string;
    /**
     * A role of the service's own namespace (e.g. "tabletsgo.admin") granted
     * at the root to whoever made the code (or admin_email), when this
     * registers the service.
     *
     * @generated from field: string admin_role = 6;
     */
    adminRole: string;
    /**
     * Who gets admin_role instead of the code's maker. Someone Hayat doesn't
     * know yet is invited (an email with a link to choose a password).
     *
     * @generated from field: string admin_email = 7;
     */
    adminEmail: string;
};
/**
 * Describes the message hayat.authz.v1.EnrollRequest.
 * Use `create(EnrollRequestSchema)` to create a new message.
 */
export declare const EnrollRequestSchema: GenMessage<EnrollRequest>;
/**
 * @generated from message hayat.authz.v1.EnrollResponse
 */
export type EnrollResponse = Message<"hayat.authz.v1.EnrollResponse"> & {
    /**
     * @generated from field: string service_id = 1;
     */
    serviceId: string;
    /**
     * The service's API credential (client credentials grant).
     *
     * @generated from field: string client_id = 2;
     */
    clientId: string;
    /**
     * @generated from field: string client_secret = 3;
     */
    clientSecret: string;
    /**
     * The public sign-in client for app_url; empty without one.
     *
     * @generated from field: string web_client_id = 4;
     */
    webClientId: string;
    /**
     * Hayat's issuer: where people sign in, and what tokens name.
     *
     * @generated from field: string issuer = 5;
     */
    issuer: string;
    /**
     * This call registered the service; otherwise it rotated the secret.
     *
     * @generated from field: bool registered = 6;
     */
    registered: boolean;
    /**
     * @generated from field: int32 contract_version = 7;
     */
    contractVersion: number;
    /**
     * "active", "pending" (it waits for other services' owners), or empty when
     * nothing was published.
     *
     * @generated from field: string contract_status = 8;
     */
    contractStatus: string;
    /**
     * What worked but deserves a look (a role not granted, a client turned off).
     *
     * @generated from field: repeated string warnings = 9;
     */
    warnings: string[];
};
/**
 * Describes the message hayat.authz.v1.EnrollResponse.
 * Use `create(EnrollResponseSchema)` to create a new message.
 */
export declare const EnrollResponseSchema: GenMessage<EnrollResponse>;
/**
 * @generated from message hayat.authz.v1.GetServerInfoRequest
 */
export type GetServerInfoRequest = Message<"hayat.authz.v1.GetServerInfoRequest"> & {};
/**
 * Describes the message hayat.authz.v1.GetServerInfoRequest.
 * Use `create(GetServerInfoRequestSchema)` to create a new message.
 */
export declare const GetServerInfoRequestSchema: GenMessage<GetServerInfoRequest>;
/**
 * @generated from message hayat.authz.v1.GetServerInfoResponse
 */
export type GetServerInfoResponse = Message<"hayat.authz.v1.GetServerInfoResponse"> & {
    /**
     * The release this binary was built from, without its leading v ("1.2.0").
     * "dev" for a build from source, which a service accepts as new enough.
     *
     * @generated from field: string version = 1;
     */
    version: string;
};
/**
 * Describes the message hayat.authz.v1.GetServerInfoResponse.
 * Use `create(GetServerInfoResponseSchema)` to create a new message.
 */
export declare const GetServerInfoResponseSchema: GenMessage<GetServerInfoResponse>;
/**
 * @generated from service hayat.authz.v1.AuthzService
 */
export declare const AuthzService: GenService<{
    /**
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.AuthzService.Check
     */
    check: {
        methodKind: "unary";
        input: typeof CheckRequestSchema;
        output: typeof CheckResponseSchema;
    };
    /**
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.AuthzService.BatchCheck
     */
    batchCheck: {
        methodKind: "unary";
        input: typeof BatchCheckRequestSchema;
        output: typeof BatchCheckResponseSchema;
    };
    /**
     * Everything a user may do at one node. For drawing a page's buttons.
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.AuthzService.PermissionsAt
     */
    permissionsAt: {
        methodKind: "unary";
        input: typeof PermissionsAtRequestSchema;
        output: typeof PermissionsAtResponseSchema;
    };
    /**
     * Nodes of one type where a user holds a permission. For lists.
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.AuthzService.LookupResources
     */
    lookupResources: {
        methodKind: "unary";
        input: typeof LookupResourcesRequestSchema;
        output: typeof LookupResourcesResponseSchema;
    };
    /**
     * People who really hold a permission at a node, through every route.
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.AuthzService.LookupSubjects
     */
    lookupSubjects: {
        methodKind: "unary";
        input: typeof LookupSubjectsRequestSchema;
        output: typeof LookupSubjectsResponseSchema;
    };
}>;
/**
 * @generated from service hayat.authz.v1.TreeService
 */
export declare const TreeService: GenService<{
    /**
     * Create or update the node mirroring a resource of yours. Idempotent: the
     * same (type, external_id) renames, re-owns or re-files the node.
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.TreeService.UpsertResource
     */
    upsertResource: {
        methodKind: "unary";
        input: typeof UpsertResourceRequestSchema;
        output: typeof UpsertResourceResponseSchema;
    };
    /**
     * Refused (FAILED_PRECONDITION) while anything is filed under it.
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.TreeService.DeleteResource
     */
    deleteResource: {
        methodKind: "unary";
        input: typeof DeleteResourceRequestSchema;
        output: typeof DeleteResourceResponseSchema;
    };
    /**
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.TreeService.GetResource
     */
    getResource: {
        methodKind: "unary";
        input: typeof GetResourceRequestSchema;
        output: typeof GetResourceResponseSchema;
    };
}>;
/**
 * @generated from service hayat.authz.v1.DirectoryService
 */
export declare const DirectoryService: GenService<{
    /**
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.DirectoryService.GetUser
     */
    getUser: {
        methodKind: "unary";
        input: typeof GetUserRequestSchema;
        output: typeof GetUserResponseSchema;
    };
    /**
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.DirectoryService.BatchGetUsers
     */
    batchGetUsers: {
        methodKind: "unary";
        input: typeof BatchGetUsersRequestSchema;
        output: typeof BatchGetUsersResponseSchema;
    };
    /**
     * The tenants a user belongs to, by name. For a service's tenant switcher.
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.DirectoryService.ListUserTenants
     */
    listUserTenants: {
        methodKind: "unary";
        input: typeof ListUserTenantsRequestSchema;
        output: typeof ListUserTenantsResponseSchema;
    };
    /**
     * A tenant's roster with each person's profile. Belonging, not permission:
     * ask PermissionsAt for what someone may do.
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.DirectoryService.ListTenantMembers
     */
    listTenantMembers: {
        methodKind: "unary";
        input: typeof ListTenantMembersRequestSchema;
        output: typeof ListTenantMembersResponseSchema;
    };
    /**
     * What each member was given in a tenant: grants made to them, to a group
     * they are on, or to the whole tenant, and the nodes they own. What was
     * granted, not what it adds up to: ask PermissionsAt for that. Only the
     * tenant, its groups and the calling service's own resource types are listed.
     * Since 1.1.0.
     *
     * @generated from rpc hayat.authz.v1.DirectoryService.ListTenantGrants
     */
    listTenantGrants: {
        methodKind: "unary";
        input: typeof ListTenantGrantsRequestSchema;
        output: typeof ListTenantGrantsResponseSchema;
    };
    /**
     * How the calling service looks for a person: the theme they gave this
     * service of its own, or else their account's, which every other app
     * follows. Both are chosen in Hayat's console. Apply it to their session;
     * don't keep a copy, so a change shows on their next page load.
     * Since 1.1.0.
     *
     * @generated from rpc hayat.authz.v1.DirectoryService.GetUserAppearance
     */
    getUserAppearance: {
        methodKind: "unary";
        input: typeof GetUserAppearanceRequestSchema;
        output: typeof GetUserAppearanceResponseSchema;
    };
}>;
/**
 * @generated from service hayat.authz.v1.WatchService
 */
export declare const WatchService: GenService<{
    /**
     * Streams every policy change from the cursor on. Drop cached decisions for
     * the event's tenant (all of them for the zero tenant: a global change).
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.WatchService.Watch
     */
    watch: {
        methodKind: "server_streaming";
        input: typeof WatchRequestSchema;
        output: typeof WatchResponseSchema;
    };
}>;
/**
 * Agents are members that aren't people: they hold roles and work in
 * services, but never sign in. Each lives in one tenant and answers to a
 * person there (its sponsor); if the sponsor leaves, it pauses. A service sees
 * only the agents that work in it, and may act as one with an agent token.
 *
 * @generated from service hayat.authz.v1.AgentService
 */
export declare const AgentService: GenService<{
    /**
     * Since 1.2.0.
     *
     * @generated from rpc hayat.authz.v1.AgentService.GetAgent
     */
    getAgent: {
        methodKind: "unary";
        input: typeof GetAgentRequestSchema;
        output: typeof GetAgentResponseSchema;
    };
    /**
     * The tenant's agents that work in the calling service.
     * Since 1.2.0.
     *
     * @generated from rpc hayat.authz.v1.AgentService.ListTenantAgents
     */
    listTenantAgents: {
        methodKind: "unary";
        input: typeof ListTenantAgentsRequestSchema;
        output: typeof ListTenantAgentsResponseSchema;
    };
    /**
     * A short-lived token to act as the agent: sub "agent:<id>", aud the calling
     * service. Refused unless the agent and its sponsor are active and the agent
     * works in the calling service. Check every action as the agent (agent_id).
     * Since 1.2.0.
     *
     * @generated from rpc hayat.authz.v1.AgentService.IssueAgentToken
     */
    issueAgentToken: {
        methodKind: "unary";
        input: typeof IssueAgentTokenRequestSchema;
        output: typeof IssueAgentTokenResponseSchema;
    };
}>;
/**
 * @generated from service hayat.authz.v1.SchemaService
 */
export declare const SchemaService: GenService<{
    /**
     * Publish a new version of your contract. It waits for owner approval.
     * Since 1.0.0.
     *
     * @generated from rpc hayat.authz.v1.SchemaService.PublishContract
     */
    publishContract: {
        methodKind: "unary";
        input: typeof PublishContractRequestSchema;
        output: typeof PublishContractResponseSchema;
    };
}>;
/**
 * The one service that takes no access token: the enrollment code is the
 * credential. A service's deployment is given a code an owner made in Hayat's
 * console, calls Enroll once, and keeps what comes back. A code for a service
 * Hayat doesn't know registers it; a code for one it knows rotates its API
 * secret (the old one stops working at once). A code is single use; one that
 * fails is given back, so the caller can fix its request and retry.
 *
 * @generated from service hayat.authz.v1.EnrollmentService
 */
export declare const EnrollmentService: GenService<{
    /**
     * What a code would do, without spending it: a setup page shows this before
     * the person confirms.
     * Since 1.2.0.
     *
     * @generated from rpc hayat.authz.v1.EnrollmentService.InspectEnrollment
     */
    inspectEnrollment: {
        methodKind: "unary";
        input: typeof InspectEnrollmentRequestSchema;
        output: typeof InspectEnrollmentResponseSchema;
    };
    /**
     * Since 1.2.0.
     *
     * @generated from rpc hayat.authz.v1.EnrollmentService.Enroll
     */
    enroll: {
        methodKind: "unary";
        input: typeof EnrollRequestSchema;
        output: typeof EnrollResponseSchema;
    };
}>;
/**
 * Served without an access token, like EnrollmentService: a service checks the
 * Hayat it reached before it enrolls or starts, and refuses one older than the
 * minimum it declares. A Hayat without this service is older than 1.2.0.
 *
 * @generated from service hayat.authz.v1.ServerInfoService
 */
export declare const ServerInfoService: GenService<{
    /**
     * Since 1.2.0.
     *
     * @generated from rpc hayat.authz.v1.ServerInfoService.GetServerInfo
     */
    getServerInfo: {
        methodKind: "unary";
        input: typeof GetServerInfoRequestSchema;
        output: typeof GetServerInfoResponseSchema;
    };
}>;
