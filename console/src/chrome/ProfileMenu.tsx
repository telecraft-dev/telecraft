import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import { demoMode } from '../api/demo'
import { Button, buttonClass } from '../ui/Button'
import { Icon } from '../ui/Icon'
import { DemoBanner } from './DemoBanner'
import { ThemeControl } from './ThemeControl'

/**
 * The profile control (issue #182): one compact button at the right edge of
 * the chrome, holding what used to sit flat in the bar. The trigger is a
 * person icon in both modes, and the button carries the accessible name:
 * signed in, the reader's name, with the panel holding the name in full,
 * the theme choice, and Sign out. On the demo there is no session to end,
 * so the button says what the demo is instead and the panel carries the
 * demo's own statement of itself (issue #50), with the theme choice beside
 * it.
 *
 * This is a hand-rolled disclosure rather than a Radix popover on purpose.
 * The popover primitive brings a floating positioner that measured 11.3 kB
 * gzipped on the entry chunk, most of the budget headroom that exists to
 * fund Tours and chrome growth (issue #125), and it buys nothing here: the
 * trigger is pinned to the chrome's right edge, so the panel is placed from
 * the trigger's rectangle at the moment it opens. The panel is fixed rather
 * than absolute because the chrome scrolls horizontally once the bar is
 * narrower than its content, and a scroll container clips an absolutely
 * positioned child; a fixed child escapes the clip. The position can go
 * stale if the viewport changes underneath an open panel, so a resize
 * closes it.
 */
export function ProfileMenu() {
  const me = useQuery({ queryKey: ['me'], queryFn: api.me })
  const queryClient = useQueryClient()
  const signOut = useMutation({
    mutationFn: api.logout,
    // Dropping the me query flips the auth gate back to the sign-in
    // surface; everything else is stale with it.
    onSuccess: () => queryClient.resetQueries(),
  })
  const [open, setOpen] = useState(false)
  const [place, setPlace] = useState({ top: 0, right: 0 })
  const wrapperRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!open) return
    const onPointerDown = (event: PointerEvent) => {
      if (!wrapperRef.current?.contains(event.target as Node)) setOpen(false)
    }
    const onResize = () => setOpen(false)
    document.addEventListener('pointerdown', onPointerDown)
    window.addEventListener('resize', onResize)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      window.removeEventListener('resize', onResize)
    }
  }, [open])

  const name = me.data?.name
  if (!demoMode && name === undefined) return null

  const toggle = () => {
    if (open) {
      setOpen(false)
      return
    }
    const rect = triggerRef.current?.getBoundingClientRect()
    if (!rect) return
    setPlace({ top: rect.bottom + 6, right: Math.max(window.innerWidth - rect.right, 12) })
    setOpen(true)
  }

  return (
    <div
      className="profile"
      ref={wrapperRef}
      onKeyDown={(event) => {
        if (event.key === 'Escape' && open) {
          // The chrome's Escape, not the Tour's: a reader closing this
          // panel has not asked to leave a running Tour.
          event.stopPropagation()
          setOpen(false)
          triggerRef.current?.focus()
        }
      }}
    >
      <button
        type="button"
        ref={triggerRef}
        className={buttonClass('secondary', 'profile-trigger')}
        data-testid="profile-trigger"
        aria-expanded={open}
        aria-controls="profile-menu"
        aria-label={demoMode ? 'Read-only demo' : `Signed in as ${name}`}
        title={demoMode ? 'Read-only demo' : `Signed in as ${name}`}
        onClick={toggle}
      >
        <Icon name="profile" size={18} />
      </button>
      {open && (
        <div
          className="profile-menu"
          id="profile-menu"
          style={{ top: place.top, right: place.right }}
        >
          {name !== undefined && (
            <span className="profile-name" data-testid="chrome-user">
              {name}
            </span>
          )}
          {demoMode && <DemoBanner />}
          <ThemeControl />
          {!demoMode && (
            <Button data-testid="sign-out" onClick={() => signOut.mutate()}>
              Sign out
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
