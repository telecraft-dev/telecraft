import * as Dialog from '@radix-ui/react-dialog'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { IndexedObject } from '../api/types'
import { deepLinkFor } from '../objectref'
import { Button } from '../ui/Button'
import { Icon } from '../ui/Icon'

/**
 * Global jump-to-object search (ADR-0042 §1): object access is served
 * here, never by object-first navigation. Matches authored objects by
 * name or id and deep-links into the Workspace that reads them.
 */
export function JumpToObject() {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const objects = useQuery({ queryKey: ['objects'], queryFn: api.objects })
  const navigate = useNavigate()

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === 'k') {
        event.preventDefault()
        setOpen((was) => !was)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const needle = query.trim().toLowerCase()
  const matches = (objects.data ?? []).filter(
    (obj) =>
      needle !== '' &&
      (obj.name.toLowerCase().includes(needle) || obj.id.toLowerCase().includes(needle)),
  )

  const jump = (obj: IndexedObject) => {
    const link = deepLinkFor(obj)
    setOpen(false)
    setQuery('')
    setActive(0)
    void navigate({
      to: link.to,
      search: (prev) => ({ lens: prev.lens, object: link.object }),
    })
  }

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) {
          setQuery('')
          setActive(0)
        }
      }}
    >
      <Dialog.Trigger asChild>
        <Button className="jump-trigger" data-testid="jump-trigger" data-tour="jump">
          <Icon name="search" />
          Jump to object
          {/* Drawn, not typed: Atkinson Hyperlegible does not contain
              U+2318, so a typed command key renders from whichever fallback
              face the reader's machine picks (ADR-0047 §6). The shortcut
              itself takes either modifier. */}
          <kbd>
            <Icon name="command" />K
          </kbd>
        </Button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="jump-overlay" />
        <Dialog.Content className="jump-dialog" aria-describedby={undefined}>
          <Dialog.Title className="jump-title">Jump to object</Dialog.Title>
          <input
            autoFocus
            data-testid="jump-input"
            className="jump-input"
            placeholder="Search Tiers, Services, Blueprints, Components, teams, Catalogue entries"
            value={query}
            onChange={(event) => {
              setQuery(event.target.value)
              setActive(0)
            }}
            onKeyDown={(event) => {
              if (event.key === 'ArrowDown') {
                event.preventDefault()
                setActive((i) => Math.min(i + 1, matches.length - 1))
              } else if (event.key === 'ArrowUp') {
                event.preventDefault()
                setActive((i) => Math.max(i - 1, 0))
              } else if (event.key === 'Enter' && matches[active]) {
                event.preventDefault()
                jump(matches[active])
              }
            }}
          />
          <ul className="jump-results" data-testid="jump-results">
            {matches.map((obj, i) => (
              <li key={`${obj.kind}:${obj.id}`}>
                <button
                  type="button"
                  data-testid={`jump-result-${obj.kind}-${obj.id}`}
                  className={i === active ? 'jump-result active' : 'jump-result'}
                  onClick={() => jump(obj)}
                >
                  <span className="jump-kind">{obj.kind}</span>
                  <span className="jump-name">{obj.name}</span>
                  <span className="jump-id">{obj.id}</span>
                </button>
              </li>
            ))}
            {needle !== '' && matches.length === 0 && (
              <li className="jump-empty">No authored object matches "{query}"</li>
            )}
          </ul>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
