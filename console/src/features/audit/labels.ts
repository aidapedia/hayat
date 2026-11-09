// Audit actions in words a person reads, not the dotted keys the server stores.
const LABELS: Record<string, string> = {
  'user.registered': 'signed up',
  'user.invited': 'invited a user',
  'user.verified': 'verified their email and set a password',
  'user.created_admin': 'created the first admin',
  'user.password_reset': 'reset their password',
  'user.password_changed': 'changed their password',
  'user.profile_updated': 'updated a profile',
  'user.status_changed': 'changed an account status',
  'user.role_changed': 'changed an instance admin role',
  'session.created': 'signed in',
  'session.revoked': 'ended a session',
  'session.revoked_all': 'signed a user out everywhere',
  'tenant.created': 'created a tenant',
  'tenant.renamed': 'renamed a tenant',
  'tenant.deleted': 'deleted a tenant',
  'tenant.member_added': 'added a tenant member',
  'tenant.member_removed': 'removed a tenant member',
  'tenant.ownership_transferred': 'transferred tenant ownership',
  'client.created': 'registered an OAuth client',
  'client.updated': 'updated an OAuth client',
  'client.secret_rotated': 'rotated a client secret',
  'client.disabled_changed': 'turned an OAuth client on or off',
  'keys.rotated': 'rotated the signing keys',
}

export const actionLabel = (action: string) => LABELS[action] ?? action

export function actorLabel(actor: string, name: string) {
  if (name) return name
  if (actor === 'system') return 'Hayat'
  if (actor === 'setup') return 'Setup'
  return actor
}
