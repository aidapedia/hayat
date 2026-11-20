import { createContext, useContext, useMemo } from 'react'
import { Code, ConnectError, createClient, type Client, type Interceptor, type Transport } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import type { DescService } from '@bufbuild/protobuf'

/**
 * Every backend call goes through one transport: the admin API on the
 * console's own origin, with the signed-in user's access token attached.
 * An Unauthenticated answer means the Hayat session ended, so the user is
 * sent to sign in again. A sensitive change refused for want of a recent
 * confirmation (step-up) asks the person to confirm it's them on Hayat.
 */
export function createTransport(getToken: () => Promise<string | null>, onSignedOut: () => void, onStepUp: () => void): Transport {
  const auth: Interceptor = (next) => async (req) => {
    const token = await getToken()
    if (token) req.header.set('Authorization', `Bearer ${token}`)
    try {
      return await next(req)
    } catch (err) {
      if (err instanceof ConnectError && err.code === Code.Unauthenticated) onSignedOut()
      if (isStepUp(err)) onStepUp()
      throw err
    }
  }
  return createConnectTransport({ baseUrl: window.location.origin, interceptors: [auth], useBinaryFormat: false })
}

export const TransportContext = createContext<Transport | null>(null)

export function useClient<T extends DescService>(service: T): Client<T> {
  const transport = useContext(TransportContext)
  if (!transport) throw new Error('useClient outside TransportContext')
  return useMemo(() => createClient(service, transport), [service, transport])
}

/** A refusal the person clears by confirming it's them (see AccountService). */
export function isStepUp(err: unknown): boolean {
  return err instanceof ConnectError && err.code === Code.PermissionDenied && err.metadata.get('hayat-step-up') === 'required'
}

/** The message to show a person for a failed call. */
export function errorMessage(err: unknown): string {
  if (err instanceof ConnectError) return err.rawMessage || 'Something went wrong.'
  if (err instanceof Error) return err.message
  return 'Something went wrong.'
}
