import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { api } from '../api/client'
import { buttonClass } from '../ui/Button'
import { BrandMark } from '../ui/BrandMark'

/**
 * The sign-in surface (REQ-017, ADR-0019 §1): rendered by the auth gate
 * whenever /api/v1/me answers 401, in place of the shell, so the URL the
 * user arrived on survives the sign-in and deep links keep working.
 *
 * What it renders is the instance's own answer to /api/v1/auth/providers:
 * a credential form for a password provider (basic auth), a link through
 * `/api/v1/auth/{name}/start` for each redirect provider (OIDC and SAML).
 * Both redirect shapes are one control here, because the instance answers
 * the flow and never the protocol. An air-gapped instance lists only what
 * it runs; no external host is ever involved (REQ-006).
 */
export function Login() {
  const providers = useQuery({ queryKey: ['auth-providers'], queryFn: api.authProviders })
  const queryClient = useQueryClient()
  const [username, setUsername] = useState('')
  const [secret, setSecret] = useState('')

  const signIn = useMutation({
    mutationFn: ({ provider }: { provider: string }) => api.login(provider, username, secret),
    onSuccess: (me) => {
      queryClient.setQueryData(['me'], me)
      // Surfaces never fetched while signed out; drop anything stale.
      void queryClient.invalidateQueries()
    },
  })

  if (providers.isPending) return <p className="surface-status">Loading sign-in…</p>
  if (providers.isError) return <p className="surface-status">Sign-in is unavailable.</p>

  const password = providers.data.find((p) => p.flow === 'password')
  const redirects = providers.data.filter((p) => p.flow === 'redirect')
  const returnTo = window.location.pathname + window.location.search

  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (password) signIn.mutate({ provider: password.name })
  }

  return (
    <main className="login">
      <section className="login-card" data-testid="login">
        <h1 className="brand">
          <BrandMark />
          Telecraft
        </h1>
        <p className="login-lede">Sign in to your estate.</p>
        {password && (
          <form className="login-form" onSubmit={submit}>
            <label>
              Email
              <input
                type="email"
                autoComplete="username"
                data-testid="login-username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
              />
            </label>
            <label>
              Password
              <input
                type="password"
                autoComplete="current-password"
                data-testid="login-secret"
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
                required
              />
            </label>
            {signIn.isError && (
              <p className="login-error" data-testid="login-error">
                That sign-in didn't work. Check the email and password.
              </p>
            )}
            <button
              type="submit"
              className={buttonClass('primary')}
              data-testid="login-submit"
              disabled={signIn.isPending}
            >
              Sign in
            </button>
          </form>
        )}
        {redirects.map((p) => (
          <a
            key={p.name}
            className={buttonClass('secondary', 'login-redirect')}
            data-testid={`login-${p.name}`}
            href={`/api/v1/auth/${encodeURIComponent(p.name)}/start?return_to=${encodeURIComponent(returnTo)}`}
          >
            Continue with {p.name.toUpperCase()}
          </a>
        ))}
      </section>
    </main>
  )
}
