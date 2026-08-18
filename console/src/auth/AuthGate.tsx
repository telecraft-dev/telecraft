import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { api, UnauthenticatedError } from '../api/client'
import { Login } from './Login'

/**
 * The gate every Workspace sits behind: until /api/v1/me answers, nothing
 * else fetches; a 401 renders the sign-in surface in place of the shell
 * (the URL survives, so the deep link the user followed resumes after
 * sign-in, ADR-0042 §3.5); any other failure is stated, not retried into.
 */
export function AuthGate({ children }: { children: ReactNode }) {
  const me = useQuery({
    queryKey: ['me'],
    queryFn: api.me,
    retry: (failureCount, error) =>
      !(error instanceof UnauthenticatedError) && failureCount < 1,
  })

  if (me.isPending) return <p className="surface-status">Loading…</p>
  if (me.isError) {
    if (me.error instanceof UnauthenticatedError) return <Login />
    return <p className="surface-status">The platform API is unreachable.</p>
  }
  return children
}
