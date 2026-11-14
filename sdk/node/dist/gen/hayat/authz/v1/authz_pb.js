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
export const file_hayat_authz_v1_authz = /*@__PURE__*/ fileDesc("ChpoYXlhdC9hdXRoei92MS9hdXRoei5wcm90bxIOaGF5YXQuYXV0aHoudjEiQQoLUmVzb3VyY2VSZWYSDAoEdHlwZRgBIAEoCRITCgtleHRlcm5hbF9pZBgCIAEoCRIPCgdub2RlX2lkGAMgASgJIj0KC0NvbnNpc3RlbmN5EhQKDGF0X2xlYXN0X3JldhgBIAEoAxIYChBmdWxseV9jb25zaXN0ZW50GAIgASgIIpcBCgZSZWFzb24SDAoEa2luZBgBIAEoCRIPCgdub2RlX2lkGAIgASgJEhEKCW5vZGVfdHlwZRgDIAEoCRIRCglub2RlX25hbWUYBCABKAkSDAoEcm9sZRgFIAEoCRIWCg5wcmluY2lwYWxfdHlwZRgGIAEoCRIUCgxwcmluY2lwYWxfaWQYByABKAkSDAoEdGV4dBgIIAEoCSKOAQoIUmVzb3VyY2USDwoHbm9kZV9pZBgBIAEoCRIMCgR0eXBlGAIgASgJEhMKC2V4dGVybmFsX2lkGAMgASgJEgwKBG5hbWUYBCABKAkSEQoJdGVuYW50X2lkGAUgASgJEhYKDnBhcmVudF9ub2RlX2lkGAYgASgJEhUKDW93bmVyX3VzZXJfaWQYByABKAkipQEKDENoZWNrUmVxdWVzdBIPCgd1c2VyX2lkGAEgASgJEhIKCnBlcm1pc3Npb24YAiABKAkSLQoIcmVzb3VyY2UYAyABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZhIwCgtjb25zaXN0ZW5jeRgEIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5Eg8KB2V4cGxhaW4YBSABKAgiaAoNQ2hlY2tSZXNwb25zZRIPCgdhbGxvd2VkGAEgASgIEgsKA3JldhgCIAEoAxIRCgl0ZW5hbnRfaWQYAyABKAkSJgoGcmVhc29uGAQgASgLMhYuaGF5YXQuYXV0aHoudjEuUmVhc29uIkEKEUJhdGNoQ2hlY2tSZXF1ZXN0EiwKBmNoZWNrcxgBIAMoCzIcLmhheWF0LmF1dGh6LnYxLkNoZWNrUmVxdWVzdCJEChJCYXRjaENoZWNrUmVzcG9uc2USLgoHcmVzdWx0cxgBIAMoCzIdLmhheWF0LmF1dGh6LnYxLkNoZWNrUmVzcG9uc2UiiAEKFFBlcm1pc3Npb25zQXRSZXF1ZXN0Eg8KB3VzZXJfaWQYASABKAkSLQoIcmVzb3VyY2UYAiABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZhIwCgtjb25zaXN0ZW5jeRgDIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5IkwKFVBlcm1pc3Npb25zQXRSZXNwb25zZRITCgtwZXJtaXNzaW9ucxgBIAMoCRILCgNyZXYYAiABKAMSEQoJdGVuYW50X2lkGAMgASgJIrcBChZMb29rdXBSZXNvdXJjZXNSZXF1ZXN0Eg8KB3VzZXJfaWQYASABKAkSEgoKcGVybWlzc2lvbhgCIAEoCRIMCgR0eXBlGAMgASgJEhEKCXRlbmFudF9pZBgEIAEoCRIRCglwYWdlX3NpemUYBSABKAUSEgoKcGFnZV90b2tlbhgGIAEoCRIwCgtjb25zaXN0ZW5jeRgHIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5Il8KF0xvb2t1cFJlc291cmNlc1Jlc3BvbnNlEisKCXJlc291cmNlcxgBIAMoCzIYLmhheWF0LmF1dGh6LnYxLlJlc291cmNlEhcKD25leHRfcGFnZV90b2tlbhgCIAEoCSKMAQoVTG9va3VwU3ViamVjdHNSZXF1ZXN0EhIKCnBlcm1pc3Npb24YASABKAkSLQoIcmVzb3VyY2UYAiABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZhIwCgtjb25zaXN0ZW5jeRgDIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5IkIKB1N1YmplY3QSDwoHdXNlcl9pZBgBIAEoCRImCgZyZWFzb24YAiABKAsyFi5oYXlhdC5hdXRoei52MS5SZWFzb24iQwoWTG9va3VwU3ViamVjdHNSZXNwb25zZRIpCghzdWJqZWN0cxgBIAMoCzIXLmhheWF0LmF1dGh6LnYxLlN1YmplY3QijAEKFVVwc2VydFJlc291cmNlUmVxdWVzdBIMCgR0eXBlGAEgASgJEhMKC2V4dGVybmFsX2lkGAIgASgJEgwKBG5hbWUYAyABKAkSKwoGcGFyZW50GAQgASgLMhsuaGF5YXQuYXV0aHoudjEuUmVzb3VyY2VSZWYSFQoNb3duZXJfdXNlcl9pZBgFIAEoCSI6ChVEZWxldGVSZXNvdXJjZVJlcXVlc3QSDAoEdHlwZRgBIAEoCRITCgtleHRlcm5hbF9pZBgCIAEoCSJDChJHZXRSZXNvdXJjZVJlcXVlc3QSLQoIcmVzb3VyY2UYASABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZiJkChZVcHNlcnRSZXNvdXJjZVJlc3BvbnNlEgsKA3JldhgBIAEoAxIRCgl0ZW5hbnRfaWQYAiABKAkSKgoIcmVzb3VyY2UYAyABKAsyGC5oYXlhdC5hdXRoei52MS5SZXNvdXJjZSI4ChZEZWxldGVSZXNvdXJjZVJlc3BvbnNlEgsKA3JldhgBIAEoAxIRCgl0ZW5hbnRfaWQYAiABKAkiQQoTR2V0UmVzb3VyY2VSZXNwb25zZRIqCghyZXNvdXJjZRgBIAEoCzIYLmhheWF0LmF1dGh6LnYxLlJlc291cmNlIu4DCgpBcHBlYXJhbmNlEg4KBnByZXNldBgBIAEoCRIMCgRtb2RlGAIgASgJEg4KBmFjY2VudBgDIAEoCRISCgpiYWNrZ3JvdW5kGAQgASgJEg8KB3N1cmZhY2UYBSABKAkSDAoEdGV4dBgGIAEoCRINCgVtdXRlZBgHIAEoCRIOCgZib3JkZXIYCCABKAkSHQoVc2lkZWJhcl9tYXRjaF9jb250ZW50GAkgASgIEhoKEnNpZGViYXJfYmFja2dyb3VuZBgKIAEoCRIUCgxzaWRlYmFyX3RleHQYCyABKAkSFgoOc2lkZWJhcl9hY3RpdmUYDCABKAkSFgoOc2lkZWJhcl9ib3JkZXIYDSABKAkSDwoHc3VjY2VzcxgOIAEoCRIPCgd3YXJuaW5nGA8gASgJEhMKC2Rlc3RydWN0aXZlGBAgASgJEgwKBGZvbnQYESABKAkSFgoJdGV4dF9zaXplGBIgASgFSACIAQESDwoHZGVuc2l0eRgTIAEoCRITCgZyYWRpdXMYFCABKAVIAYgBARIaCg1zaWRlYmFyX3dpZHRoGBUgASgFSAKIAQESFQoNY29udGVudF93aWR0aBgWIAEoCUIMCgpfdGV4dF9zaXplQgkKB19yYWRpdXNCEAoOX3NpZGViYXJfd2lkdGgiKwoYR2V0VXNlckFwcGVhcmFuY2VSZXF1ZXN0Eg8KB3VzZXJfaWQYASABKAkiSwoZR2V0VXNlckFwcGVhcmFuY2VSZXNwb25zZRIuCgphcHBlYXJhbmNlGAEgASgLMhouaGF5YXQuYXV0aHoudjEuQXBwZWFyYW5jZSJuCgZUZW5hbnQSCgoCaWQYASABKAkSDAoEc2x1ZxgCIAEoCRIMCgRuYW1lGAMgASgJEhUKDW93bmVyX3VzZXJfaWQYBCABKAkSDwoHbm9kZV9pZBgFIAEoCRIUCgxtZW1iZXJfY291bnQYBiABKAUiKQoWTGlzdFVzZXJUZW5hbnRzUmVxdWVzdBIPCgd1c2VyX2lkGAEgASgJIkIKF0xpc3RVc2VyVGVuYW50c1Jlc3BvbnNlEicKB3RlbmFudHMYASADKAsyFi5oYXlhdC5hdXRoei52MS5UZW5hbnQiLQoYTGlzdFRlbmFudE1lbWJlcnNSZXF1ZXN0EhEKCXRlbmFudF9pZBgBIAEoCSJzCgxUZW5hbnRNZW1iZXISIgoEdXNlchgBIAEoCzIULmhheWF0LmF1dGh6LnYxLlVzZXISEAoIaXNfb3duZXIYAiABKAgSLQoJam9pbmVkX2F0GAMgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcCJKChlMaXN0VGVuYW50TWVtYmVyc1Jlc3BvbnNlEi0KB21lbWJlcnMYASADKAsyHC5oYXlhdC5hdXRoei52MS5UZW5hbnRNZW1iZXIiPgoXTGlzdFRlbmFudEdyYW50c1JlcXVlc3QSEQoJdGVuYW50X2lkGAEgASgJEhAKCHVzZXJfaWRzGAIgAygJIq8BCgtNZW1iZXJHcmFudBIPCgd1c2VyX2lkGAEgASgJEioKCHJlc291cmNlGAIgASgLMhguaGF5YXQuYXV0aHoudjEuUmVzb3VyY2USCwoDdmlhGAMgASgJEgwKBHJvbGUYBCABKAkSEQoJcm9sZV9uYW1lGAUgASgJEg8KB2luaGVyaXQYBiABKAgSEAoIZ3JvdXBfaWQYByABKAkSEgoKZ3JvdXBfbmFtZRgIIAEoCSJHChhMaXN0VGVuYW50R3JhbnRzUmVzcG9uc2USKwoGZ3JhbnRzGAEgAygLMhsuaGF5YXQuYXV0aHoudjEuTWVtYmVyR3JhbnQiigEKBFVzZXISCgoCaWQYASABKAkSDQoFZW1haWwYAiABKAkSFgoOZW1haWxfdmVyaWZpZWQYAyABKAgSDAoEbmFtZRgEIAEoCRIPCgdwaWN0dXJlGAUgASgJEg4KBmxvY2FsZRgGIAEoCRIQCgh6b25laW5mbxgHIAEoCRIOCgZzdGF0dXMYCCABKAkiHAoOR2V0VXNlclJlcXVlc3QSCgoCaWQYASABKAkiNQoPR2V0VXNlclJlc3BvbnNlEiIKBHVzZXIYASABKAsyFC5oYXlhdC5hdXRoei52MS5Vc2VyIiMKFEJhdGNoR2V0VXNlcnNSZXF1ZXN0EgsKA2lkcxgBIAMoCSI8ChVCYXRjaEdldFVzZXJzUmVzcG9uc2USIwoFdXNlcnMYASADKAsyFC5oYXlhdC5hdXRoei52MS5Vc2VyIh4KDFdhdGNoUmVxdWVzdBIOCgZjdXJzb3IYASABKAkiOwoNV2F0Y2hSZXNwb25zZRIqCgVldmVudBgBIAEoCzIbLmhheWF0LmF1dGh6LnYxLlBvbGljeUV2ZW50Im8KC1BvbGljeUV2ZW50EgoKAmlkGAEgASgJEhEKCXRlbmFudF9pZBgCIAEoCRILCgNyZXYYAyABKAMSDAoEa2luZBgEIAEoCRImCgJhdBgFIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXAiKgoWUHVibGlzaENvbnRyYWN0UmVxdWVzdBIQCghtYW5pZmVzdBgBIAEoCSJOChdQdWJsaXNoQ29udHJhY3RSZXNwb25zZRIPCgd2ZXJzaW9uGAEgASgFEg4KBnN0YXR1cxgCIAEoCRISCgpuYW1lc3BhY2VzGAMgAygJMswDCgxBdXRoelNlcnZpY2USRAoFQ2hlY2sSHC5oYXlhdC5hdXRoei52MS5DaGVja1JlcXVlc3QaHS5oYXlhdC5hdXRoei52MS5DaGVja1Jlc3BvbnNlElMKCkJhdGNoQ2hlY2sSIS5oYXlhdC5hdXRoei52MS5CYXRjaENoZWNrUmVxdWVzdBoiLmhheWF0LmF1dGh6LnYxLkJhdGNoQ2hlY2tSZXNwb25zZRJcCg1QZXJtaXNzaW9uc0F0EiQuaGF5YXQuYXV0aHoudjEuUGVybWlzc2lvbnNBdFJlcXVlc3QaJS5oYXlhdC5hdXRoei52MS5QZXJtaXNzaW9uc0F0UmVzcG9uc2USYgoPTG9va3VwUmVzb3VyY2VzEiYuaGF5YXQuYXV0aHoudjEuTG9va3VwUmVzb3VyY2VzUmVxdWVzdBonLmhheWF0LmF1dGh6LnYxLkxvb2t1cFJlc291cmNlc1Jlc3BvbnNlEl8KDkxvb2t1cFN1YmplY3RzEiUuaGF5YXQuYXV0aHoudjEuTG9va3VwU3ViamVjdHNSZXF1ZXN0GiYuaGF5YXQuYXV0aHoudjEuTG9va3VwU3ViamVjdHNSZXNwb25zZTKnAgoLVHJlZVNlcnZpY2USXwoOVXBzZXJ0UmVzb3VyY2USJS5oYXlhdC5hdXRoei52MS5VcHNlcnRSZXNvdXJjZVJlcXVlc3QaJi5oYXlhdC5hdXRoei52MS5VcHNlcnRSZXNvdXJjZVJlc3BvbnNlEl8KDkRlbGV0ZVJlc291cmNlEiUuaGF5YXQuYXV0aHoudjEuRGVsZXRlUmVzb3VyY2VSZXF1ZXN0GiYuaGF5YXQuYXV0aHoudjEuRGVsZXRlUmVzb3VyY2VSZXNwb25zZRJWCgtHZXRSZXNvdXJjZRIiLmhheWF0LmF1dGh6LnYxLkdldFJlc291cmNlUmVxdWVzdBojLmhheWF0LmF1dGh6LnYxLkdldFJlc291cmNlUmVzcG9uc2Uy2wQKEERpcmVjdG9yeVNlcnZpY2USSgoHR2V0VXNlchIeLmhheWF0LmF1dGh6LnYxLkdldFVzZXJSZXF1ZXN0Gh8uaGF5YXQuYXV0aHoudjEuR2V0VXNlclJlc3BvbnNlElwKDUJhdGNoR2V0VXNlcnMSJC5oYXlhdC5hdXRoei52MS5CYXRjaEdldFVzZXJzUmVxdWVzdBolLmhheWF0LmF1dGh6LnYxLkJhdGNoR2V0VXNlcnNSZXNwb25zZRJiCg9MaXN0VXNlclRlbmFudHMSJi5oYXlhdC5hdXRoei52MS5MaXN0VXNlclRlbmFudHNSZXF1ZXN0GicuaGF5YXQuYXV0aHoudjEuTGlzdFVzZXJUZW5hbnRzUmVzcG9uc2USaAoRTGlzdFRlbmFudE1lbWJlcnMSKC5oYXlhdC5hdXRoei52MS5MaXN0VGVuYW50TWVtYmVyc1JlcXVlc3QaKS5oYXlhdC5hdXRoei52MS5MaXN0VGVuYW50TWVtYmVyc1Jlc3BvbnNlEmUKEExpc3RUZW5hbnRHcmFudHMSJy5oYXlhdC5hdXRoei52MS5MaXN0VGVuYW50R3JhbnRzUmVxdWVzdBooLmhheWF0LmF1dGh6LnYxLkxpc3RUZW5hbnRHcmFudHNSZXNwb25zZRJoChFHZXRVc2VyQXBwZWFyYW5jZRIoLmhheWF0LmF1dGh6LnYxLkdldFVzZXJBcHBlYXJhbmNlUmVxdWVzdBopLmhheWF0LmF1dGh6LnYxLkdldFVzZXJBcHBlYXJhbmNlUmVzcG9uc2UyVgoMV2F0Y2hTZXJ2aWNlEkYKBVdhdGNoEhwuaGF5YXQuYXV0aHoudjEuV2F0Y2hSZXF1ZXN0Gh0uaGF5YXQuYXV0aHoudjEuV2F0Y2hSZXNwb25zZTABMnMKDVNjaGVtYVNlcnZpY2USYgoPUHVibGlzaENvbnRyYWN0EiYuaGF5YXQuYXV0aHoudjEuUHVibGlzaENvbnRyYWN0UmVxdWVzdBonLmhheWF0LmF1dGh6LnYxLlB1Ymxpc2hDb250cmFjdFJlc3BvbnNlQjpaOGdpdGh1Yi5jb20vYWlkYXBlZGlhL2hheWF0L2dlbi9nby9oYXlhdC9hdXRoei92MTthdXRoenYxYgZwcm90bzM", [file_google_protobuf_timestamp]);
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
 * Describes the message hayat.authz.v1.Appearance.
 * Use `create(AppearanceSchema)` to create a new message.
 */
export const AppearanceSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 21);
/**
 * Describes the message hayat.authz.v1.GetUserAppearanceRequest.
 * Use `create(GetUserAppearanceRequestSchema)` to create a new message.
 */
export const GetUserAppearanceRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 22);
/**
 * Describes the message hayat.authz.v1.GetUserAppearanceResponse.
 * Use `create(GetUserAppearanceResponseSchema)` to create a new message.
 */
export const GetUserAppearanceResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 23);
/**
 * Describes the message hayat.authz.v1.Tenant.
 * Use `create(TenantSchema)` to create a new message.
 */
export const TenantSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 24);
/**
 * Describes the message hayat.authz.v1.ListUserTenantsRequest.
 * Use `create(ListUserTenantsRequestSchema)` to create a new message.
 */
export const ListUserTenantsRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 25);
/**
 * Describes the message hayat.authz.v1.ListUserTenantsResponse.
 * Use `create(ListUserTenantsResponseSchema)` to create a new message.
 */
export const ListUserTenantsResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 26);
/**
 * Describes the message hayat.authz.v1.ListTenantMembersRequest.
 * Use `create(ListTenantMembersRequestSchema)` to create a new message.
 */
export const ListTenantMembersRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 27);
/**
 * Describes the message hayat.authz.v1.TenantMember.
 * Use `create(TenantMemberSchema)` to create a new message.
 */
export const TenantMemberSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 28);
/**
 * Describes the message hayat.authz.v1.ListTenantMembersResponse.
 * Use `create(ListTenantMembersResponseSchema)` to create a new message.
 */
export const ListTenantMembersResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 29);
/**
 * Describes the message hayat.authz.v1.ListTenantGrantsRequest.
 * Use `create(ListTenantGrantsRequestSchema)` to create a new message.
 */
export const ListTenantGrantsRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 30);
/**
 * Describes the message hayat.authz.v1.MemberGrant.
 * Use `create(MemberGrantSchema)` to create a new message.
 */
export const MemberGrantSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 31);
/**
 * Describes the message hayat.authz.v1.ListTenantGrantsResponse.
 * Use `create(ListTenantGrantsResponseSchema)` to create a new message.
 */
export const ListTenantGrantsResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 32);
/**
 * Describes the message hayat.authz.v1.User.
 * Use `create(UserSchema)` to create a new message.
 */
export const UserSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 33);
/**
 * Describes the message hayat.authz.v1.GetUserRequest.
 * Use `create(GetUserRequestSchema)` to create a new message.
 */
export const GetUserRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 34);
/**
 * Describes the message hayat.authz.v1.GetUserResponse.
 * Use `create(GetUserResponseSchema)` to create a new message.
 */
export const GetUserResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 35);
/**
 * Describes the message hayat.authz.v1.BatchGetUsersRequest.
 * Use `create(BatchGetUsersRequestSchema)` to create a new message.
 */
export const BatchGetUsersRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 36);
/**
 * Describes the message hayat.authz.v1.BatchGetUsersResponse.
 * Use `create(BatchGetUsersResponseSchema)` to create a new message.
 */
export const BatchGetUsersResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 37);
/**
 * Describes the message hayat.authz.v1.WatchRequest.
 * Use `create(WatchRequestSchema)` to create a new message.
 */
export const WatchRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 38);
/**
 * Describes the message hayat.authz.v1.WatchResponse.
 * Use `create(WatchResponseSchema)` to create a new message.
 */
export const WatchResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 39);
/**
 * Describes the message hayat.authz.v1.PolicyEvent.
 * Use `create(PolicyEventSchema)` to create a new message.
 */
export const PolicyEventSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 40);
/**
 * Describes the message hayat.authz.v1.PublishContractRequest.
 * Use `create(PublishContractRequestSchema)` to create a new message.
 */
export const PublishContractRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 41);
/**
 * Describes the message hayat.authz.v1.PublishContractResponse.
 * Use `create(PublishContractResponseSchema)` to create a new message.
 */
export const PublishContractResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 42);
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
