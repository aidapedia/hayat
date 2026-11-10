// The service API: what your services call, over gRPC (or Connect/gRPC-Web)
// on the api role's port. Every call carries an access token from the client
// credentials grant, issued to the OAuth client a registered service owns.
//
// A service asks about any user (it is trusted to ask on their behalf), but
// writes only nodes of its own namespace.
import { fileDesc, messageDesc, serviceDesc } from "@bufbuild/protobuf/codegenv2";
import { file_google_protobuf_timestamp } from "@bufbuild/protobuf/wkt";
/**
 * Describes the file hayat/authz/v1/authz.proto.
 */
export const file_hayat_authz_v1_authz = /*@__PURE__*/ fileDesc("ChpoYXlhdC9hdXRoei92MS9hdXRoei5wcm90bxIOaGF5YXQuYXV0aHoudjEiQQoLUmVzb3VyY2VSZWYSDAoEdHlwZRgBIAEoCRITCgtleHRlcm5hbF9pZBgCIAEoCRIPCgdub2RlX2lkGAMgASgJIj0KC0NvbnNpc3RlbmN5EhQKDGF0X2xlYXN0X3JldhgBIAEoAxIYChBmdWxseV9jb25zaXN0ZW50GAIgASgIIpcBCgZSZWFzb24SDAoEa2luZBgBIAEoCRIPCgdub2RlX2lkGAIgASgJEhEKCW5vZGVfdHlwZRgDIAEoCRIRCglub2RlX25hbWUYBCABKAkSDAoEcm9sZRgFIAEoCRIWCg5wcmluY2lwYWxfdHlwZRgGIAEoCRIUCgxwcmluY2lwYWxfaWQYByABKAkSDAoEdGV4dBgIIAEoCSKOAQoIUmVzb3VyY2USDwoHbm9kZV9pZBgBIAEoCRIMCgR0eXBlGAIgASgJEhMKC2V4dGVybmFsX2lkGAMgASgJEgwKBG5hbWUYBCABKAkSEQoJdGVuYW50X2lkGAUgASgJEhYKDnBhcmVudF9ub2RlX2lkGAYgASgJEhUKDW93bmVyX3VzZXJfaWQYByABKAkipQEKDENoZWNrUmVxdWVzdBIPCgd1c2VyX2lkGAEgASgJEhIKCnBlcm1pc3Npb24YAiABKAkSLQoIcmVzb3VyY2UYAyABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZhIwCgtjb25zaXN0ZW5jeRgEIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5Eg8KB2V4cGxhaW4YBSABKAgiaAoNQ2hlY2tSZXNwb25zZRIPCgdhbGxvd2VkGAEgASgIEgsKA3JldhgCIAEoAxIRCgl0ZW5hbnRfaWQYAyABKAkSJgoGcmVhc29uGAQgASgLMhYuaGF5YXQuYXV0aHoudjEuUmVhc29uIkEKEUJhdGNoQ2hlY2tSZXF1ZXN0EiwKBmNoZWNrcxgBIAMoCzIcLmhheWF0LmF1dGh6LnYxLkNoZWNrUmVxdWVzdCJEChJCYXRjaENoZWNrUmVzcG9uc2USLgoHcmVzdWx0cxgBIAMoCzIdLmhheWF0LmF1dGh6LnYxLkNoZWNrUmVzcG9uc2UiiAEKFFBlcm1pc3Npb25zQXRSZXF1ZXN0Eg8KB3VzZXJfaWQYASABKAkSLQoIcmVzb3VyY2UYAiABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZhIwCgtjb25zaXN0ZW5jeRgDIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5IkwKFVBlcm1pc3Npb25zQXRSZXNwb25zZRITCgtwZXJtaXNzaW9ucxgBIAMoCRILCgNyZXYYAiABKAMSEQoJdGVuYW50X2lkGAMgASgJIrcBChZMb29rdXBSZXNvdXJjZXNSZXF1ZXN0Eg8KB3VzZXJfaWQYASABKAkSEgoKcGVybWlzc2lvbhgCIAEoCRIMCgR0eXBlGAMgASgJEhEKCXRlbmFudF9pZBgEIAEoCRIRCglwYWdlX3NpemUYBSABKAUSEgoKcGFnZV90b2tlbhgGIAEoCRIwCgtjb25zaXN0ZW5jeRgHIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5Il8KF0xvb2t1cFJlc291cmNlc1Jlc3BvbnNlEisKCXJlc291cmNlcxgBIAMoCzIYLmhheWF0LmF1dGh6LnYxLlJlc291cmNlEhcKD25leHRfcGFnZV90b2tlbhgCIAEoCSKMAQoVTG9va3VwU3ViamVjdHNSZXF1ZXN0EhIKCnBlcm1pc3Npb24YASABKAkSLQoIcmVzb3VyY2UYAiABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZhIwCgtjb25zaXN0ZW5jeRgDIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5IkIKB1N1YmplY3QSDwoHdXNlcl9pZBgBIAEoCRImCgZyZWFzb24YAiABKAsyFi5oYXlhdC5hdXRoei52MS5SZWFzb24iQwoWTG9va3VwU3ViamVjdHNSZXNwb25zZRIpCghzdWJqZWN0cxgBIAMoCzIXLmhheWF0LmF1dGh6LnYxLlN1YmplY3QijAEKFVVwc2VydFJlc291cmNlUmVxdWVzdBIMCgR0eXBlGAEgASgJEhMKC2V4dGVybmFsX2lkGAIgASgJEgwKBG5hbWUYAyABKAkSKwoGcGFyZW50GAQgASgLMhsuaGF5YXQuYXV0aHoudjEuUmVzb3VyY2VSZWYSFQoNb3duZXJfdXNlcl9pZBgFIAEoCSI6ChVEZWxldGVSZXNvdXJjZVJlcXVlc3QSDAoEdHlwZRgBIAEoCRITCgtleHRlcm5hbF9pZBgCIAEoCSJDChJHZXRSZXNvdXJjZVJlcXVlc3QSLQoIcmVzb3VyY2UYASABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZiJkChZVcHNlcnRSZXNvdXJjZVJlc3BvbnNlEgsKA3JldhgBIAEoAxIRCgl0ZW5hbnRfaWQYAiABKAkSKgoIcmVzb3VyY2UYAyABKAsyGC5oYXlhdC5hdXRoei52MS5SZXNvdXJjZSI4ChZEZWxldGVSZXNvdXJjZVJlc3BvbnNlEgsKA3JldhgBIAEoAxIRCgl0ZW5hbnRfaWQYAiABKAkiQQoTR2V0UmVzb3VyY2VSZXNwb25zZRIqCghyZXNvdXJjZRgBIAEoCzIYLmhheWF0LmF1dGh6LnYxLlJlc291cmNlIm4KBlRlbmFudBIKCgJpZBgBIAEoCRIMCgRzbHVnGAIgASgJEgwKBG5hbWUYAyABKAkSFQoNb3duZXJfdXNlcl9pZBgEIAEoCRIPCgdub2RlX2lkGAUgASgJEhQKDG1lbWJlcl9jb3VudBgGIAEoBSIpChZMaXN0VXNlclRlbmFudHNSZXF1ZXN0Eg8KB3VzZXJfaWQYASABKAkiQgoXTGlzdFVzZXJUZW5hbnRzUmVzcG9uc2USJwoHdGVuYW50cxgBIAMoCzIWLmhheWF0LmF1dGh6LnYxLlRlbmFudCItChhMaXN0VGVuYW50TWVtYmVyc1JlcXVlc3QSEQoJdGVuYW50X2lkGAEgASgJInMKDFRlbmFudE1lbWJlchIiCgR1c2VyGAEgASgLMhQuaGF5YXQuYXV0aHoudjEuVXNlchIQCghpc19vd25lchgCIAEoCBItCglqb2luZWRfYXQYAyABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wIkoKGUxpc3RUZW5hbnRNZW1iZXJzUmVzcG9uc2USLQoHbWVtYmVycxgBIAMoCzIcLmhheWF0LmF1dGh6LnYxLlRlbmFudE1lbWJlciKKAQoEVXNlchIKCgJpZBgBIAEoCRINCgVlbWFpbBgCIAEoCRIWCg5lbWFpbF92ZXJpZmllZBgDIAEoCBIMCgRuYW1lGAQgASgJEg8KB3BpY3R1cmUYBSABKAkSDgoGbG9jYWxlGAYgASgJEhAKCHpvbmVpbmZvGAcgASgJEg4KBnN0YXR1cxgIIAEoCSIcCg5HZXRVc2VyUmVxdWVzdBIKCgJpZBgBIAEoCSI1Cg9HZXRVc2VyUmVzcG9uc2USIgoEdXNlchgBIAEoCzIULmhheWF0LmF1dGh6LnYxLlVzZXIiIwoUQmF0Y2hHZXRVc2Vyc1JlcXVlc3QSCwoDaWRzGAEgAygJIjwKFUJhdGNoR2V0VXNlcnNSZXNwb25zZRIjCgV1c2VycxgBIAMoCzIULmhheWF0LmF1dGh6LnYxLlVzZXIiHgoMV2F0Y2hSZXF1ZXN0Eg4KBmN1cnNvchgBIAEoCSI7Cg1XYXRjaFJlc3BvbnNlEioKBWV2ZW50GAEgASgLMhsuaGF5YXQuYXV0aHoudjEuUG9saWN5RXZlbnQibwoLUG9saWN5RXZlbnQSCgoCaWQYASABKAkSEQoJdGVuYW50X2lkGAIgASgJEgsKA3JldhgDIAEoAxIMCgRraW5kGAQgASgJEiYKAmF0GAUgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcCIqChZQdWJsaXNoQ29udHJhY3RSZXF1ZXN0EhAKCG1hbmlmZXN0GAEgASgJIk4KF1B1Ymxpc2hDb250cmFjdFJlc3BvbnNlEg8KB3ZlcnNpb24YASABKAUSDgoGc3RhdHVzGAIgASgJEhIKCm5hbWVzcGFjZXMYAyADKAkyzAMKDEF1dGh6U2VydmljZRJECgVDaGVjaxIcLmhheWF0LmF1dGh6LnYxLkNoZWNrUmVxdWVzdBodLmhheWF0LmF1dGh6LnYxLkNoZWNrUmVzcG9uc2USUwoKQmF0Y2hDaGVjaxIhLmhheWF0LmF1dGh6LnYxLkJhdGNoQ2hlY2tSZXF1ZXN0GiIuaGF5YXQuYXV0aHoudjEuQmF0Y2hDaGVja1Jlc3BvbnNlElwKDVBlcm1pc3Npb25zQXQSJC5oYXlhdC5hdXRoei52MS5QZXJtaXNzaW9uc0F0UmVxdWVzdBolLmhheWF0LmF1dGh6LnYxLlBlcm1pc3Npb25zQXRSZXNwb25zZRJiCg9Mb29rdXBSZXNvdXJjZXMSJi5oYXlhdC5hdXRoei52MS5Mb29rdXBSZXNvdXJjZXNSZXF1ZXN0GicuaGF5YXQuYXV0aHoudjEuTG9va3VwUmVzb3VyY2VzUmVzcG9uc2USXwoOTG9va3VwU3ViamVjdHMSJS5oYXlhdC5hdXRoei52MS5Mb29rdXBTdWJqZWN0c1JlcXVlc3QaJi5oYXlhdC5hdXRoei52MS5Mb29rdXBTdWJqZWN0c1Jlc3BvbnNlMqcCCgtUcmVlU2VydmljZRJfCg5VcHNlcnRSZXNvdXJjZRIlLmhheWF0LmF1dGh6LnYxLlVwc2VydFJlc291cmNlUmVxdWVzdBomLmhheWF0LmF1dGh6LnYxLlVwc2VydFJlc291cmNlUmVzcG9uc2USXwoORGVsZXRlUmVzb3VyY2USJS5oYXlhdC5hdXRoei52MS5EZWxldGVSZXNvdXJjZVJlcXVlc3QaJi5oYXlhdC5hdXRoei52MS5EZWxldGVSZXNvdXJjZVJlc3BvbnNlElYKC0dldFJlc291cmNlEiIuaGF5YXQuYXV0aHoudjEuR2V0UmVzb3VyY2VSZXF1ZXN0GiMuaGF5YXQuYXV0aHoudjEuR2V0UmVzb3VyY2VSZXNwb25zZTKKAwoQRGlyZWN0b3J5U2VydmljZRJKCgdHZXRVc2VyEh4uaGF5YXQuYXV0aHoudjEuR2V0VXNlclJlcXVlc3QaHy5oYXlhdC5hdXRoei52MS5HZXRVc2VyUmVzcG9uc2USXAoNQmF0Y2hHZXRVc2VycxIkLmhheWF0LmF1dGh6LnYxLkJhdGNoR2V0VXNlcnNSZXF1ZXN0GiUuaGF5YXQuYXV0aHoudjEuQmF0Y2hHZXRVc2Vyc1Jlc3BvbnNlEmIKD0xpc3RVc2VyVGVuYW50cxImLmhheWF0LmF1dGh6LnYxLkxpc3RVc2VyVGVuYW50c1JlcXVlc3QaJy5oYXlhdC5hdXRoei52MS5MaXN0VXNlclRlbmFudHNSZXNwb25zZRJoChFMaXN0VGVuYW50TWVtYmVycxIoLmhheWF0LmF1dGh6LnYxLkxpc3RUZW5hbnRNZW1iZXJzUmVxdWVzdBopLmhheWF0LmF1dGh6LnYxLkxpc3RUZW5hbnRNZW1iZXJzUmVzcG9uc2UyVgoMV2F0Y2hTZXJ2aWNlEkYKBVdhdGNoEhwuaGF5YXQuYXV0aHoudjEuV2F0Y2hSZXF1ZXN0Gh0uaGF5YXQuYXV0aHoudjEuV2F0Y2hSZXNwb25zZTABMnMKDVNjaGVtYVNlcnZpY2USYgoPUHVibGlzaENvbnRyYWN0EiYuaGF5YXQuYXV0aHoudjEuUHVibGlzaENvbnRyYWN0UmVxdWVzdBonLmhheWF0LmF1dGh6LnYxLlB1Ymxpc2hDb250cmFjdFJlc3BvbnNlQjpaOGdpdGh1Yi5jb20vYWlkYXBlZGlhL2hheWF0L2dlbi9nby9oYXlhdC9hdXRoei92MTthdXRoenYxYgZwcm90bzM", [file_google_protobuf_timestamp]);
/**
 * Describes the message hayat.authz.v1.ResourceRef.
 * Use `create(ResourceRefSchema)` to create a new message.
 */
export const ResourceRefSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 0);
/**
 * Describes the message hayat.authz.v1.Consistency.
 * Use `create(ConsistencySchema)` to create a new message.
 */
export const ConsistencySchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 1);
/**
 * Describes the message hayat.authz.v1.Reason.
 * Use `create(ReasonSchema)` to create a new message.
 */
export const ReasonSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 2);
/**
 * Describes the message hayat.authz.v1.Resource.
 * Use `create(ResourceSchema)` to create a new message.
 */
export const ResourceSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 3);
/**
 * Describes the message hayat.authz.v1.CheckRequest.
 * Use `create(CheckRequestSchema)` to create a new message.
 */
export const CheckRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 4);
/**
 * Describes the message hayat.authz.v1.CheckResponse.
 * Use `create(CheckResponseSchema)` to create a new message.
 */
export const CheckResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 5);
/**
 * Describes the message hayat.authz.v1.BatchCheckRequest.
 * Use `create(BatchCheckRequestSchema)` to create a new message.
 */
export const BatchCheckRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 6);
/**
 * Describes the message hayat.authz.v1.BatchCheckResponse.
 * Use `create(BatchCheckResponseSchema)` to create a new message.
 */
export const BatchCheckResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 7);
/**
 * Describes the message hayat.authz.v1.PermissionsAtRequest.
 * Use `create(PermissionsAtRequestSchema)` to create a new message.
 */
export const PermissionsAtRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 8);
/**
 * Describes the message hayat.authz.v1.PermissionsAtResponse.
 * Use `create(PermissionsAtResponseSchema)` to create a new message.
 */
export const PermissionsAtResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 9);
/**
 * Describes the message hayat.authz.v1.LookupResourcesRequest.
 * Use `create(LookupResourcesRequestSchema)` to create a new message.
 */
export const LookupResourcesRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 10);
/**
 * Describes the message hayat.authz.v1.LookupResourcesResponse.
 * Use `create(LookupResourcesResponseSchema)` to create a new message.
 */
export const LookupResourcesResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 11);
/**
 * Describes the message hayat.authz.v1.LookupSubjectsRequest.
 * Use `create(LookupSubjectsRequestSchema)` to create a new message.
 */
export const LookupSubjectsRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 12);
/**
 * Describes the message hayat.authz.v1.Subject.
 * Use `create(SubjectSchema)` to create a new message.
 */
export const SubjectSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 13);
/**
 * Describes the message hayat.authz.v1.LookupSubjectsResponse.
 * Use `create(LookupSubjectsResponseSchema)` to create a new message.
 */
export const LookupSubjectsResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 14);
/**
 * Describes the message hayat.authz.v1.UpsertResourceRequest.
 * Use `create(UpsertResourceRequestSchema)` to create a new message.
 */
export const UpsertResourceRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 15);
/**
 * Describes the message hayat.authz.v1.DeleteResourceRequest.
 * Use `create(DeleteResourceRequestSchema)` to create a new message.
 */
export const DeleteResourceRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 16);
/**
 * Describes the message hayat.authz.v1.GetResourceRequest.
 * Use `create(GetResourceRequestSchema)` to create a new message.
 */
export const GetResourceRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 17);
/**
 * Describes the message hayat.authz.v1.UpsertResourceResponse.
 * Use `create(UpsertResourceResponseSchema)` to create a new message.
 */
export const UpsertResourceResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 18);
/**
 * Describes the message hayat.authz.v1.DeleteResourceResponse.
 * Use `create(DeleteResourceResponseSchema)` to create a new message.
 */
export const DeleteResourceResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 19);
/**
 * Describes the message hayat.authz.v1.GetResourceResponse.
 * Use `create(GetResourceResponseSchema)` to create a new message.
 */
export const GetResourceResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 20);
/**
 * Describes the message hayat.authz.v1.Tenant.
 * Use `create(TenantSchema)` to create a new message.
 */
export const TenantSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 21);
/**
 * Describes the message hayat.authz.v1.ListUserTenantsRequest.
 * Use `create(ListUserTenantsRequestSchema)` to create a new message.
 */
export const ListUserTenantsRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 22);
/**
 * Describes the message hayat.authz.v1.ListUserTenantsResponse.
 * Use `create(ListUserTenantsResponseSchema)` to create a new message.
 */
export const ListUserTenantsResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 23);
/**
 * Describes the message hayat.authz.v1.ListTenantMembersRequest.
 * Use `create(ListTenantMembersRequestSchema)` to create a new message.
 */
export const ListTenantMembersRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 24);
/**
 * Describes the message hayat.authz.v1.TenantMember.
 * Use `create(TenantMemberSchema)` to create a new message.
 */
export const TenantMemberSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 25);
/**
 * Describes the message hayat.authz.v1.ListTenantMembersResponse.
 * Use `create(ListTenantMembersResponseSchema)` to create a new message.
 */
export const ListTenantMembersResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 26);
/**
 * Describes the message hayat.authz.v1.User.
 * Use `create(UserSchema)` to create a new message.
 */
export const UserSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 27);
/**
 * Describes the message hayat.authz.v1.GetUserRequest.
 * Use `create(GetUserRequestSchema)` to create a new message.
 */
export const GetUserRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 28);
/**
 * Describes the message hayat.authz.v1.GetUserResponse.
 * Use `create(GetUserResponseSchema)` to create a new message.
 */
export const GetUserResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 29);
/**
 * Describes the message hayat.authz.v1.BatchGetUsersRequest.
 * Use `create(BatchGetUsersRequestSchema)` to create a new message.
 */
export const BatchGetUsersRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 30);
/**
 * Describes the message hayat.authz.v1.BatchGetUsersResponse.
 * Use `create(BatchGetUsersResponseSchema)` to create a new message.
 */
export const BatchGetUsersResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 31);
/**
 * Describes the message hayat.authz.v1.WatchRequest.
 * Use `create(WatchRequestSchema)` to create a new message.
 */
export const WatchRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 32);
/**
 * Describes the message hayat.authz.v1.WatchResponse.
 * Use `create(WatchResponseSchema)` to create a new message.
 */
export const WatchResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 33);
/**
 * Describes the message hayat.authz.v1.PolicyEvent.
 * Use `create(PolicyEventSchema)` to create a new message.
 */
export const PolicyEventSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 34);
/**
 * Describes the message hayat.authz.v1.PublishContractRequest.
 * Use `create(PublishContractRequestSchema)` to create a new message.
 */
export const PublishContractRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 35);
/**
 * Describes the message hayat.authz.v1.PublishContractResponse.
 * Use `create(PublishContractResponseSchema)` to create a new message.
 */
export const PublishContractResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 36);
/**
 * @generated from service hayat.authz.v1.AuthzService
 */
export const AuthzService = /*@__PURE__*/ serviceDesc(file_hayat_authz_v1_authz, 0);
/**
 * @generated from service hayat.authz.v1.TreeService
 */
export const TreeService = /*@__PURE__*/ serviceDesc(file_hayat_authz_v1_authz, 1);
/**
 * @generated from service hayat.authz.v1.DirectoryService
 */
export const DirectoryService = /*@__PURE__*/ serviceDesc(file_hayat_authz_v1_authz, 2);
/**
 * @generated from service hayat.authz.v1.WatchService
 */
export const WatchService = /*@__PURE__*/ serviceDesc(file_hayat_authz_v1_authz, 3);
/**
 * @generated from service hayat.authz.v1.SchemaService
 */
export const SchemaService = /*@__PURE__*/ serviceDesc(file_hayat_authz_v1_authz, 4);
