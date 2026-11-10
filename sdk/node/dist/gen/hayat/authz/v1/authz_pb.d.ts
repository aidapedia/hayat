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
     * @generated from field: string user_id = 1;
     */
    userId: string;
    /**
     * @generated from field: hayat.authz.v1.Reason reason = 2;
     */
    reason?: Reason | undefined;
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
 * @generated from service hayat.authz.v1.AuthzService
 */
export declare const AuthzService: GenService<{
    /**
     * @generated from rpc hayat.authz.v1.AuthzService.Check
     */
    check: {
        methodKind: "unary";
        input: typeof CheckRequestSchema;
        output: typeof CheckResponseSchema;
    };
    /**
     * @generated from rpc hayat.authz.v1.AuthzService.BatchCheck
     */
    batchCheck: {
        methodKind: "unary";
        input: typeof BatchCheckRequestSchema;
        output: typeof BatchCheckResponseSchema;
    };
    /**
     * Everything a user may do at one node. For drawing a page's buttons.
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
     *
     * @generated from rpc hayat.authz.v1.TreeService.DeleteResource
     */
    deleteResource: {
        methodKind: "unary";
        input: typeof DeleteResourceRequestSchema;
        output: typeof DeleteResourceResponseSchema;
    };
    /**
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
     * @generated from rpc hayat.authz.v1.DirectoryService.GetUser
     */
    getUser: {
        methodKind: "unary";
        input: typeof GetUserRequestSchema;
        output: typeof GetUserResponseSchema;
    };
    /**
     * @generated from rpc hayat.authz.v1.DirectoryService.BatchGetUsers
     */
    batchGetUsers: {
        methodKind: "unary";
        input: typeof BatchGetUsersRequestSchema;
        output: typeof BatchGetUsersResponseSchema;
    };
    /**
     * The tenants a user belongs to, by name. For a service's tenant switcher.
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
     *
     * @generated from rpc hayat.authz.v1.DirectoryService.ListTenantMembers
     */
    listTenantMembers: {
        methodKind: "unary";
        input: typeof ListTenantMembersRequestSchema;
        output: typeof ListTenantMembersResponseSchema;
    };
}>;
/**
 * @generated from service hayat.authz.v1.WatchService
 */
export declare const WatchService: GenService<{
    /**
     * Streams every policy change from the cursor on. Drop cached decisions for
     * the event's tenant (all of them for the zero tenant: a global change).
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
 * @generated from service hayat.authz.v1.SchemaService
 */
export declare const SchemaService: GenService<{
    /**
     * Publish a new version of your contract. It waits for owner approval.
     *
     * @generated from rpc hayat.authz.v1.SchemaService.PublishContract
     */
    publishContract: {
        methodKind: "unary";
        input: typeof PublishContractRequestSchema;
        output: typeof PublishContractResponseSchema;
    };
}>;
