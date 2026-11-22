// The service API: what your services call, over gRPC (or Connect/gRPC-Web)
// on the api role's port. Every call carries an access token from the client
// credentials grant, issued to the OAuth client a registered service owns —
// except EnrollmentService and ServerInfoService, which a service calls before
// it has credentials.
//
// A service asks about any user (it is trusted to ask on their behalf), but
// writes only nodes of its own namespace.
//
// Versioning. This package only grows: a release adds services, RPCs and
// fields, and never removes, renames or renumbers one (`buf breaking` in CI
// holds it to that), so a newer Hayat serves every older service. Every RPC
// says the Hayat release it first shipped in ("Since 1.2.0."); a service's
// minimum Hayat is the newest Since among the RPCs it calls, never lower than
// 1.2.0 — the first release that reports its version through
// ServerInfoService. The SDKs refuse to start without that minimum and check
// it against the Hayat they reach.
import { fileDesc, messageDesc, serviceDesc } from "@bufbuild/protobuf/codegenv2";
import { file_google_protobuf_timestamp } from "@bufbuild/protobuf/wkt";
/**
 * Describes the file hayat/authz/v1/authz.proto.
 */
export const file_hayat_authz_v1_authz = /*@__PURE__*/ fileDesc("ChpoYXlhdC9hdXRoei92MS9hdXRoei5wcm90bxIOaGF5YXQuYXV0aHoudjEiQQoLUmVzb3VyY2VSZWYSDAoEdHlwZRgBIAEoCRITCgtleHRlcm5hbF9pZBgCIAEoCRIPCgdub2RlX2lkGAMgASgJIj0KC0NvbnNpc3RlbmN5EhQKDGF0X2xlYXN0X3JldhgBIAEoAxIYChBmdWxseV9jb25zaXN0ZW50GAIgASgIIpcBCgZSZWFzb24SDAoEa2luZBgBIAEoCRIPCgdub2RlX2lkGAIgASgJEhEKCW5vZGVfdHlwZRgDIAEoCRIRCglub2RlX25hbWUYBCABKAkSDAoEcm9sZRgFIAEoCRIWCg5wcmluY2lwYWxfdHlwZRgGIAEoCRIUCgxwcmluY2lwYWxfaWQYByABKAkSDAoEdGV4dBgIIAEoCSKOAQoIUmVzb3VyY2USDwoHbm9kZV9pZBgBIAEoCRIMCgR0eXBlGAIgASgJEhMKC2V4dGVybmFsX2lkGAMgASgJEgwKBG5hbWUYBCABKAkSEQoJdGVuYW50X2lkGAUgASgJEhYKDnBhcmVudF9ub2RlX2lkGAYgASgJEhUKDW93bmVyX3VzZXJfaWQYByABKAkitwEKDENoZWNrUmVxdWVzdBIPCgd1c2VyX2lkGAEgASgJEhIKCnBlcm1pc3Npb24YAiABKAkSLQoIcmVzb3VyY2UYAyABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZhIwCgtjb25zaXN0ZW5jeRgEIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5Eg8KB2V4cGxhaW4YBSABKAgSEAoIYWdlbnRfaWQYBiABKAkiaAoNQ2hlY2tSZXNwb25zZRIPCgdhbGxvd2VkGAEgASgIEgsKA3JldhgCIAEoAxIRCgl0ZW5hbnRfaWQYAyABKAkSJgoGcmVhc29uGAQgASgLMhYuaGF5YXQuYXV0aHoudjEuUmVhc29uIkEKEUJhdGNoQ2hlY2tSZXF1ZXN0EiwKBmNoZWNrcxgBIAMoCzIcLmhheWF0LmF1dGh6LnYxLkNoZWNrUmVxdWVzdCJEChJCYXRjaENoZWNrUmVzcG9uc2USLgoHcmVzdWx0cxgBIAMoCzIdLmhheWF0LmF1dGh6LnYxLkNoZWNrUmVzcG9uc2UimgEKFFBlcm1pc3Npb25zQXRSZXF1ZXN0Eg8KB3VzZXJfaWQYASABKAkSLQoIcmVzb3VyY2UYAiABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZhIwCgtjb25zaXN0ZW5jeRgDIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5EhAKCGFnZW50X2lkGAQgASgJIkwKFVBlcm1pc3Npb25zQXRSZXNwb25zZRITCgtwZXJtaXNzaW9ucxgBIAMoCRILCgNyZXYYAiABKAMSEQoJdGVuYW50X2lkGAMgASgJIskBChZMb29rdXBSZXNvdXJjZXNSZXF1ZXN0Eg8KB3VzZXJfaWQYASABKAkSEgoKcGVybWlzc2lvbhgCIAEoCRIMCgR0eXBlGAMgASgJEhEKCXRlbmFudF9pZBgEIAEoCRIRCglwYWdlX3NpemUYBSABKAUSEgoKcGFnZV90b2tlbhgGIAEoCRIwCgtjb25zaXN0ZW5jeRgHIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5EhAKCGFnZW50X2lkGAggASgJIl8KF0xvb2t1cFJlc291cmNlc1Jlc3BvbnNlEisKCXJlc291cmNlcxgBIAMoCzIYLmhheWF0LmF1dGh6LnYxLlJlc291cmNlEhcKD25leHRfcGFnZV90b2tlbhgCIAEoCSKMAQoVTG9va3VwU3ViamVjdHNSZXF1ZXN0EhIKCnBlcm1pc3Npb24YASABKAkSLQoIcmVzb3VyY2UYAiABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZhIwCgtjb25zaXN0ZW5jeRgDIAEoCzIbLmhheWF0LmF1dGh6LnYxLkNvbnNpc3RlbmN5IlQKB1N1YmplY3QSDwoHdXNlcl9pZBgBIAEoCRImCgZyZWFzb24YAiABKAsyFi5oYXlhdC5hdXRoei52MS5SZWFzb24SEAoIYWdlbnRfaWQYAyABKAkiQwoWTG9va3VwU3ViamVjdHNSZXNwb25zZRIpCghzdWJqZWN0cxgBIAMoCzIXLmhheWF0LmF1dGh6LnYxLlN1YmplY3QijAEKFVVwc2VydFJlc291cmNlUmVxdWVzdBIMCgR0eXBlGAEgASgJEhMKC2V4dGVybmFsX2lkGAIgASgJEgwKBG5hbWUYAyABKAkSKwoGcGFyZW50GAQgASgLMhsuaGF5YXQuYXV0aHoudjEuUmVzb3VyY2VSZWYSFQoNb3duZXJfdXNlcl9pZBgFIAEoCSI6ChVEZWxldGVSZXNvdXJjZVJlcXVlc3QSDAoEdHlwZRgBIAEoCRITCgtleHRlcm5hbF9pZBgCIAEoCSJDChJHZXRSZXNvdXJjZVJlcXVlc3QSLQoIcmVzb3VyY2UYASABKAsyGy5oYXlhdC5hdXRoei52MS5SZXNvdXJjZVJlZiJkChZVcHNlcnRSZXNvdXJjZVJlc3BvbnNlEgsKA3JldhgBIAEoAxIRCgl0ZW5hbnRfaWQYAiABKAkSKgoIcmVzb3VyY2UYAyABKAsyGC5oYXlhdC5hdXRoei52MS5SZXNvdXJjZSI4ChZEZWxldGVSZXNvdXJjZVJlc3BvbnNlEgsKA3JldhgBIAEoAxIRCgl0ZW5hbnRfaWQYAiABKAkiQQoTR2V0UmVzb3VyY2VSZXNwb25zZRIqCghyZXNvdXJjZRgBIAEoCzIYLmhheWF0LmF1dGh6LnYxLlJlc291cmNlIqEECgpBcHBlYXJhbmNlEg4KBnByZXNldBgBIAEoCRIMCgRtb2RlGAIgASgJEg4KBmFjY2VudBgDIAEoCRISCgpiYWNrZ3JvdW5kGAQgASgJEg8KB3N1cmZhY2UYBSABKAkSDAoEdGV4dBgGIAEoCRINCgVtdXRlZBgHIAEoCRIOCgZib3JkZXIYCCABKAkSHQoVc2lkZWJhcl9tYXRjaF9jb250ZW50GAkgASgIEhoKEnNpZGViYXJfYmFja2dyb3VuZBgKIAEoCRIUCgxzaWRlYmFyX3RleHQYCyABKAkSFgoOc2lkZWJhcl9hY3RpdmUYDCABKAkSFgoOc2lkZWJhcl9ib3JkZXIYDSABKAkSDwoHc3VjY2VzcxgOIAEoCRIPCgd3YXJuaW5nGA8gASgJEhMKC2Rlc3RydWN0aXZlGBAgASgJEgwKBGZvbnQYESABKAkSFgoJdGV4dF9zaXplGBIgASgFSACIAQESDwoHZGVuc2l0eRgTIAEoCRITCgZyYWRpdXMYFCABKAVIAYgBARIaCg1zaWRlYmFyX3dpZHRoGBUgASgFSAKIAQESFQoNY29udGVudF93aWR0aBgWIAEoCRITCgthY2NlbnRfdGV4dBgXIAEoCRISCgVzY2FsZRgYIAEoBUgDiAEBQgwKCl90ZXh0X3NpemVCCQoHX3JhZGl1c0IQCg5fc2lkZWJhcl93aWR0aEIICgZfc2NhbGUiKwoYR2V0VXNlckFwcGVhcmFuY2VSZXF1ZXN0Eg8KB3VzZXJfaWQYASABKAkiWAoZR2V0VXNlckFwcGVhcmFuY2VSZXNwb25zZRIuCgphcHBlYXJhbmNlGAEgASgLMhouaGF5YXQuYXV0aHoudjEuQXBwZWFyYW5jZRILCgNvd24YAiABKAgigAEKBlRlbmFudBIKCgJpZBgBIAEoCRIMCgRzbHVnGAIgASgJEgwKBG5hbWUYAyABKAkSFQoNb3duZXJfdXNlcl9pZBgEIAEoCRIPCgdub2RlX2lkGAUgASgJEhQKDG1lbWJlcl9jb3VudBgGIAEoBRIQCghsb2dvX3VybBgHIAEoCSIpChZMaXN0VXNlclRlbmFudHNSZXF1ZXN0Eg8KB3VzZXJfaWQYASABKAkiQgoXTGlzdFVzZXJUZW5hbnRzUmVzcG9uc2USJwoHdGVuYW50cxgBIAMoCzIWLmhheWF0LmF1dGh6LnYxLlRlbmFudCItChhMaXN0VGVuYW50TWVtYmVyc1JlcXVlc3QSEQoJdGVuYW50X2lkGAEgASgJInMKDFRlbmFudE1lbWJlchIiCgR1c2VyGAEgASgLMhQuaGF5YXQuYXV0aHoudjEuVXNlchIQCghpc19vd25lchgCIAEoCBItCglqb2luZWRfYXQYAyABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wIkoKGUxpc3RUZW5hbnRNZW1iZXJzUmVzcG9uc2USLQoHbWVtYmVycxgBIAMoCzIcLmhheWF0LmF1dGh6LnYxLlRlbmFudE1lbWJlciI+ChdMaXN0VGVuYW50R3JhbnRzUmVxdWVzdBIRCgl0ZW5hbnRfaWQYASABKAkSEAoIdXNlcl9pZHMYAiADKAkirwEKC01lbWJlckdyYW50Eg8KB3VzZXJfaWQYASABKAkSKgoIcmVzb3VyY2UYAiABKAsyGC5oYXlhdC5hdXRoei52MS5SZXNvdXJjZRILCgN2aWEYAyABKAkSDAoEcm9sZRgEIAEoCRIRCglyb2xlX25hbWUYBSABKAkSDwoHaW5oZXJpdBgGIAEoCBIQCghncm91cF9pZBgHIAEoCRISCgpncm91cF9uYW1lGAggASgJIkcKGExpc3RUZW5hbnRHcmFudHNSZXNwb25zZRIrCgZncmFudHMYASADKAsyGy5oYXlhdC5hdXRoei52MS5NZW1iZXJHcmFudCKKAQoEVXNlchIKCgJpZBgBIAEoCRINCgVlbWFpbBgCIAEoCRIWCg5lbWFpbF92ZXJpZmllZBgDIAEoCBIMCgRuYW1lGAQgASgJEg8KB3BpY3R1cmUYBSABKAkSDgoGbG9jYWxlGAYgASgJEhAKCHpvbmVpbmZvGAcgASgJEg4KBnN0YXR1cxgIIAEoCSIcCg5HZXRVc2VyUmVxdWVzdBIKCgJpZBgBIAEoCSI1Cg9HZXRVc2VyUmVzcG9uc2USIgoEdXNlchgBIAEoCzIULmhheWF0LmF1dGh6LnYxLlVzZXIiIwoUQmF0Y2hHZXRVc2Vyc1JlcXVlc3QSCwoDaWRzGAEgAygJIjwKFUJhdGNoR2V0VXNlcnNSZXNwb25zZRIjCgV1c2VycxgBIAMoCzIULmhheWF0LmF1dGh6LnYxLlVzZXIiHgoMV2F0Y2hSZXF1ZXN0Eg4KBmN1cnNvchgBIAEoCSI7Cg1XYXRjaFJlc3BvbnNlEioKBWV2ZW50GAEgASgLMhsuaGF5YXQuYXV0aHoudjEuUG9saWN5RXZlbnQibwoLUG9saWN5RXZlbnQSCgoCaWQYASABKAkSEQoJdGVuYW50X2lkGAIgASgJEgsKA3JldhgDIAEoAxIMCgRraW5kGAQgASgJEiYKAmF0GAUgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcCKpAQoFQWdlbnQSCgoCaWQYASABKAkSEQoJdGVuYW50X2lkGAIgASgJEg4KBmhhbmRsZRgDIAEoCRIMCgRuYW1lGAQgASgJEhcKD3Nwb25zb3JfdXNlcl9pZBgFIAEoCRIOCgZzdGF0dXMYBiABKAkSFQoNc3RhdHVzX3JlYXNvbhgHIAEoCRIQCghzZXJ2aWNlcxgIIAMoCRIRCglncm91cF9pZHMYCSADKAkiIwoPR2V0QWdlbnRSZXF1ZXN0EhAKCGFnZW50X2lkGAEgASgJIjgKEEdldEFnZW50UmVzcG9uc2USJAoFYWdlbnQYASABKAsyFS5oYXlhdC5hdXRoei52MS5BZ2VudCIsChdMaXN0VGVuYW50QWdlbnRzUmVxdWVzdBIRCgl0ZW5hbnRfaWQYASABKAkiQQoYTGlzdFRlbmFudEFnZW50c1Jlc3BvbnNlEiUKBmFnZW50cxgBIAMoCzIVLmhheWF0LmF1dGh6LnYxLkFnZW50IioKFklzc3VlQWdlbnRUb2tlblJlcXVlc3QSEAoIYWdlbnRfaWQYASABKAkiXwoXSXNzdWVBZ2VudFRva2VuUmVzcG9uc2USFAoMYWNjZXNzX3Rva2VuGAEgASgJEi4KCmV4cGlyZXNfYXQYAiABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wIioKFlB1Ymxpc2hDb250cmFjdFJlcXVlc3QSEAoIbWFuaWZlc3QYASABKAkiTgoXUHVibGlzaENvbnRyYWN0UmVzcG9uc2USDwoHdmVyc2lvbhgBIAEoBRIOCgZzdGF0dXMYAiABKAkSEgoKbmFtZXNwYWNlcxgDIAMoCSIoChhJbnNwZWN0RW5yb2xsbWVudFJlcXVlc3QSDAoEY29kZRgBIAEoCSLFAQoZSW5zcGVjdEVucm9sbG1lbnRSZXNwb25zZRISCgpzZXJ2aWNlX2lkGAEgASgJEhQKDGRpc3BsYXlfbmFtZRgCIAEoCRIRCglyZWdpc3RlcnMYAyABKAgSFQoNbWFkZV9ieV9lbWFpbBgEIAEoCRIUCgxtYWRlX2J5X25hbWUYBSABKAkSLgoKZXhwaXJlc19hdBgGIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASDgoGaXNzdWVyGAcgASgJIpQBCg1FbnJvbGxSZXF1ZXN0EgwKBGNvZGUYASABKAkSEAoIbWFuaWZlc3QYAiABKAkSFAoMZGlzcGxheV9uYW1lGAMgASgJEhMKC2Rlc2NyaXB0aW9uGAQgASgJEg8KB2FwcF91cmwYBSABKAkSEgoKYWRtaW5fcm9sZRgGIAEoCRITCgthZG1pbl9lbWFpbBgHIAEoCSLOAQoORW5yb2xsUmVzcG9uc2USEgoKc2VydmljZV9pZBgBIAEoCRIRCgljbGllbnRfaWQYAiABKAkSFQoNY2xpZW50X3NlY3JldBgDIAEoCRIVCg13ZWJfY2xpZW50X2lkGAQgASgJEg4KBmlzc3VlchgFIAEoCRISCgpyZWdpc3RlcmVkGAYgASgIEhgKEGNvbnRyYWN0X3ZlcnNpb24YByABKAUSFwoPY29udHJhY3Rfc3RhdHVzGAggASgJEhAKCHdhcm5pbmdzGAkgAygJIhYKFEdldFNlcnZlckluZm9SZXF1ZXN0IigKFUdldFNlcnZlckluZm9SZXNwb25zZRIPCgd2ZXJzaW9uGAEgASgJMswDCgxBdXRoelNlcnZpY2USRAoFQ2hlY2sSHC5oYXlhdC5hdXRoei52MS5DaGVja1JlcXVlc3QaHS5oYXlhdC5hdXRoei52MS5DaGVja1Jlc3BvbnNlElMKCkJhdGNoQ2hlY2sSIS5oYXlhdC5hdXRoei52MS5CYXRjaENoZWNrUmVxdWVzdBoiLmhheWF0LmF1dGh6LnYxLkJhdGNoQ2hlY2tSZXNwb25zZRJcCg1QZXJtaXNzaW9uc0F0EiQuaGF5YXQuYXV0aHoudjEuUGVybWlzc2lvbnNBdFJlcXVlc3QaJS5oYXlhdC5hdXRoei52MS5QZXJtaXNzaW9uc0F0UmVzcG9uc2USYgoPTG9va3VwUmVzb3VyY2VzEiYuaGF5YXQuYXV0aHoudjEuTG9va3VwUmVzb3VyY2VzUmVxdWVzdBonLmhheWF0LmF1dGh6LnYxLkxvb2t1cFJlc291cmNlc1Jlc3BvbnNlEl8KDkxvb2t1cFN1YmplY3RzEiUuaGF5YXQuYXV0aHoudjEuTG9va3VwU3ViamVjdHNSZXF1ZXN0GiYuaGF5YXQuYXV0aHoudjEuTG9va3VwU3ViamVjdHNSZXNwb25zZTKnAgoLVHJlZVNlcnZpY2USXwoOVXBzZXJ0UmVzb3VyY2USJS5oYXlhdC5hdXRoei52MS5VcHNlcnRSZXNvdXJjZVJlcXVlc3QaJi5oYXlhdC5hdXRoei52MS5VcHNlcnRSZXNvdXJjZVJlc3BvbnNlEl8KDkRlbGV0ZVJlc291cmNlEiUuaGF5YXQuYXV0aHoudjEuRGVsZXRlUmVzb3VyY2VSZXF1ZXN0GiYuaGF5YXQuYXV0aHoudjEuRGVsZXRlUmVzb3VyY2VSZXNwb25zZRJWCgtHZXRSZXNvdXJjZRIiLmhheWF0LmF1dGh6LnYxLkdldFJlc291cmNlUmVxdWVzdBojLmhheWF0LmF1dGh6LnYxLkdldFJlc291cmNlUmVzcG9uc2Uy2wQKEERpcmVjdG9yeVNlcnZpY2USSgoHR2V0VXNlchIeLmhheWF0LmF1dGh6LnYxLkdldFVzZXJSZXF1ZXN0Gh8uaGF5YXQuYXV0aHoudjEuR2V0VXNlclJlc3BvbnNlElwKDUJhdGNoR2V0VXNlcnMSJC5oYXlhdC5hdXRoei52MS5CYXRjaEdldFVzZXJzUmVxdWVzdBolLmhheWF0LmF1dGh6LnYxLkJhdGNoR2V0VXNlcnNSZXNwb25zZRJiCg9MaXN0VXNlclRlbmFudHMSJi5oYXlhdC5hdXRoei52MS5MaXN0VXNlclRlbmFudHNSZXF1ZXN0GicuaGF5YXQuYXV0aHoudjEuTGlzdFVzZXJUZW5hbnRzUmVzcG9uc2USaAoRTGlzdFRlbmFudE1lbWJlcnMSKC5oYXlhdC5hdXRoei52MS5MaXN0VGVuYW50TWVtYmVyc1JlcXVlc3QaKS5oYXlhdC5hdXRoei52MS5MaXN0VGVuYW50TWVtYmVyc1Jlc3BvbnNlEmUKEExpc3RUZW5hbnRHcmFudHMSJy5oYXlhdC5hdXRoei52MS5MaXN0VGVuYW50R3JhbnRzUmVxdWVzdBooLmhheWF0LmF1dGh6LnYxLkxpc3RUZW5hbnRHcmFudHNSZXNwb25zZRJoChFHZXRVc2VyQXBwZWFyYW5jZRIoLmhheWF0LmF1dGh6LnYxLkdldFVzZXJBcHBlYXJhbmNlUmVxdWVzdBopLmhheWF0LmF1dGh6LnYxLkdldFVzZXJBcHBlYXJhbmNlUmVzcG9uc2UyVgoMV2F0Y2hTZXJ2aWNlEkYKBVdhdGNoEhwuaGF5YXQuYXV0aHoudjEuV2F0Y2hSZXF1ZXN0Gh0uaGF5YXQuYXV0aHoudjEuV2F0Y2hSZXNwb25zZTABMqgCCgxBZ2VudFNlcnZpY2USTQoIR2V0QWdlbnQSHy5oYXlhdC5hdXRoei52MS5HZXRBZ2VudFJlcXVlc3QaIC5oYXlhdC5hdXRoei52MS5HZXRBZ2VudFJlc3BvbnNlEmUKEExpc3RUZW5hbnRBZ2VudHMSJy5oYXlhdC5hdXRoei52MS5MaXN0VGVuYW50QWdlbnRzUmVxdWVzdBooLmhheWF0LmF1dGh6LnYxLkxpc3RUZW5hbnRBZ2VudHNSZXNwb25zZRJiCg9Jc3N1ZUFnZW50VG9rZW4SJi5oYXlhdC5hdXRoei52MS5Jc3N1ZUFnZW50VG9rZW5SZXF1ZXN0GicuaGF5YXQuYXV0aHoudjEuSXNzdWVBZ2VudFRva2VuUmVzcG9uc2UycwoNU2NoZW1hU2VydmljZRJiCg9QdWJsaXNoQ29udHJhY3QSJi5oYXlhdC5hdXRoei52MS5QdWJsaXNoQ29udHJhY3RSZXF1ZXN0GicuaGF5YXQuYXV0aHoudjEuUHVibGlzaENvbnRyYWN0UmVzcG9uc2UyxgEKEUVucm9sbG1lbnRTZXJ2aWNlEmgKEUluc3BlY3RFbnJvbGxtZW50EiguaGF5YXQuYXV0aHoudjEuSW5zcGVjdEVucm9sbG1lbnRSZXF1ZXN0GikuaGF5YXQuYXV0aHoudjEuSW5zcGVjdEVucm9sbG1lbnRSZXNwb25zZRJHCgZFbnJvbGwSHS5oYXlhdC5hdXRoei52MS5FbnJvbGxSZXF1ZXN0Gh4uaGF5YXQuYXV0aHoudjEuRW5yb2xsUmVzcG9uc2UycQoRU2VydmVySW5mb1NlcnZpY2USXAoNR2V0U2VydmVySW5mbxIkLmhheWF0LmF1dGh6LnYxLkdldFNlcnZlckluZm9SZXF1ZXN0GiUuaGF5YXQuYXV0aHoudjEuR2V0U2VydmVySW5mb1Jlc3BvbnNlQjpaOGdpdGh1Yi5jb20vYWlkYXBlZGlhL2hheWF0L2dlbi9nby9oYXlhdC9hdXRoei92MTthdXRoenYxYgZwcm90bzM", [file_google_protobuf_timestamp]);
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
 * Describes the message hayat.authz.v1.Agent.
 * Use `create(AgentSchema)` to create a new message.
 */
export const AgentSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 41);
/**
 * Describes the message hayat.authz.v1.GetAgentRequest.
 * Use `create(GetAgentRequestSchema)` to create a new message.
 */
export const GetAgentRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 42);
/**
 * Describes the message hayat.authz.v1.GetAgentResponse.
 * Use `create(GetAgentResponseSchema)` to create a new message.
 */
export const GetAgentResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 43);
/**
 * Describes the message hayat.authz.v1.ListTenantAgentsRequest.
 * Use `create(ListTenantAgentsRequestSchema)` to create a new message.
 */
export const ListTenantAgentsRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 44);
/**
 * Describes the message hayat.authz.v1.ListTenantAgentsResponse.
 * Use `create(ListTenantAgentsResponseSchema)` to create a new message.
 */
export const ListTenantAgentsResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 45);
/**
 * Describes the message hayat.authz.v1.IssueAgentTokenRequest.
 * Use `create(IssueAgentTokenRequestSchema)` to create a new message.
 */
export const IssueAgentTokenRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 46);
/**
 * Describes the message hayat.authz.v1.IssueAgentTokenResponse.
 * Use `create(IssueAgentTokenResponseSchema)` to create a new message.
 */
export const IssueAgentTokenResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 47);
/**
 * Describes the message hayat.authz.v1.PublishContractRequest.
 * Use `create(PublishContractRequestSchema)` to create a new message.
 */
export const PublishContractRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 48);
/**
 * Describes the message hayat.authz.v1.PublishContractResponse.
 * Use `create(PublishContractResponseSchema)` to create a new message.
 */
export const PublishContractResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 49);
/**
 * Describes the message hayat.authz.v1.InspectEnrollmentRequest.
 * Use `create(InspectEnrollmentRequestSchema)` to create a new message.
 */
export const InspectEnrollmentRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 50);
/**
 * Describes the message hayat.authz.v1.InspectEnrollmentResponse.
 * Use `create(InspectEnrollmentResponseSchema)` to create a new message.
 */
export const InspectEnrollmentResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 51);
/**
 * Describes the message hayat.authz.v1.EnrollRequest.
 * Use `create(EnrollRequestSchema)` to create a new message.
 */
export const EnrollRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 52);
/**
 * Describes the message hayat.authz.v1.EnrollResponse.
 * Use `create(EnrollResponseSchema)` to create a new message.
 */
export const EnrollResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 53);
/**
 * Describes the message hayat.authz.v1.GetServerInfoRequest.
 * Use `create(GetServerInfoRequestSchema)` to create a new message.
 */
export const GetServerInfoRequestSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 54);
/**
 * Describes the message hayat.authz.v1.GetServerInfoResponse.
 * Use `create(GetServerInfoResponseSchema)` to create a new message.
 */
export const GetServerInfoResponseSchema = /*@__PURE__*/ messageDesc(file_hayat_authz_v1_authz, 55);
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
 * Agents are members that aren't people: they hold roles and work in
 * services, but never sign in. Each lives in one tenant and answers to a
 * person there (its sponsor); if the sponsor leaves, it pauses. A service sees
 * only the agents that work in it, and may act as one with an agent token.
 *
 * @generated from service hayat.authz.v1.AgentService
 */
export const AgentService = /*@__PURE__*/ serviceDesc(file_hayat_authz_v1_authz, 4);
/**
 * @generated from service hayat.authz.v1.SchemaService
 */
export const SchemaService = /*@__PURE__*/ serviceDesc(file_hayat_authz_v1_authz, 5);
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
export const EnrollmentService = /*@__PURE__*/ serviceDesc(file_hayat_authz_v1_authz, 6);
/**
 * Served without an access token, like EnrollmentService: a service checks the
 * Hayat it reached before it enrolls or starts, and refuses one older than the
 * minimum it declares. A Hayat without this service is older than 1.2.0.
 *
 * @generated from service hayat.authz.v1.ServerInfoService
 */
export const ServerInfoService = /*@__PURE__*/ serviceDesc(file_hayat_authz_v1_authz, 7);
