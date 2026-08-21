import type { Tour } from './types'

/**
 * The welcome Tour: the one every reader needs, and the only one offered
 * from the chrome. It opens once per user on a bare landing URL (ADR-0051
 * §7) and can be taken again whenever.
 *
 * It teaches *this console* — where things are, and which of them are
 * claims rather than pictures. It deliberately does not teach the product:
 * a Step that starts defining an Expectation has become documentation in
 * the wrong repository.
 *
 * The first Step is the only one that reads differently on the public demo
 * (§8), because what the reader is looking at is the only thing that
 * differs there.
 */
export const welcome: Tour = {
  id: 'welcome',
  title: 'A tour of the console',
  summary: 'A pass over the estate you are signed in to. It changes nothing.',
  steps: [
    {
      id: 'welcome',
      title: 'Welcome to Telecraft',
      body:
        'Telecraft composes collector configuration from governed building blocks, then ' +
        'derives what telemetry should arrive from that configuration and checks that it ' +
        'did. Green means the configuration worked, not merely that it applied. This tour ' +
        'runs over your own estate and changes nothing in it. Escape leaves at any point.',
      demoBody:
        'You are looking at the real console over a public demo estate, rebuilt from git on ' +
        'every push and read-only by construction. Telecraft composes collector ' +
        'configuration from governed building blocks, then checks that the telemetry that ' +
        'configuration implies actually arrived. This tour changes nothing. Escape leaves ' +
        'at any point.',
      to: '/estate',
    },
    {
      id: 'shelf',
      title: "The shelf answers 'how are we doing?'",
      body:
        'One card per Tier: a position in your collection topology, and the unit ' +
        'configuration is rendered for. The worst leads the shelf, and your own team ' +
        'subtree rests here — one click widens it to the whole estate. Nothing healthy is ' +
        'hidden away, because a hidden card reads as a healthy one.',
      anchor: 'card',
      to: '/estate',
      search: { view: 'shelf' },
    },
    {
      id: 'bands',
      title: 'Three bands, and three distinct reds',
      body:
        'Delivery asks whether the configuration ever reached the collector. Expectation ' +
        'asks whether the telemetry it implied arrived. Conformance asks whether that ' +
        'telemetry is right. Three different failures with three different owners, so each ' +
        'band keeps its own mark and its own word. The colour only reinforces them.',
      anchor: 'card-bands',
      to: '/estate',
      search: { view: 'shelf' },
    },
    {
      id: 'lens',
      title: 'The environment lens',
      body:
        'Production leads by default. The lens sets emphasis and the evaluation context; it ' +
        'is never a filter, so a staging problem cannot vanish because you were looking at ' +
        'production. Explicit filters stay available on the flat list, where per-collector ' +
        'detail lives.',
      anchor: 'lens',
    },
    {
      id: 'topology',
      title: 'How telemetry flows',
      body:
        "Tiers, the hops between them, and each Service's path through the graph. " +
        'Collectors are never drawn: they match into a Tier by selector and appear as its ' +
        'count. Edges are derived from the configuration rather than arranged by hand, so ' +
        'the picture cannot disagree with what is rendered.',
      anchor: 'tier-node',
      to: '/topology',
      search: { view: 'flow' },
    },
    {
      id: 'compose',
      title: 'Authoring leaves as a pull request',
      body:
        'Compose opens one Blueprint at a time and judges every edit live against what your ' +
        'team is allowed to use. Saving writes nothing: it opens a pull request carrying the ' +
        'rendered configuration, attributed to you. The console proposes and the pull request ' +
        'decides, which is why git stays the source of truth.',
      anchor: 'compose',
      to: '/compose',
    },
    {
      id: 'catalogue',
      title: 'What you may use, and how to ask for more',
      body:
        'The catalogue states what exists at a given collector version. Your effective ' +
        'palette is that narrowed by the allow-list your team inherits, and every entry ' +
        'carries the provenance of why it is there. Asking for something outside it is a ' +
        'Grant, and a Grant is another pull request.',
      anchor: 'catalogue',
      to: '/catalogue',
      search: { view: 'browse' },
    },
    {
      id: 'urls',
      title: 'Everything is one URL away',
      body:
        'Jump to any Tier, Service, Blueprint or Component from here, or with Command-K. ' +
        'Every state you can reach is in the address bar — the view, the selection, the ' +
        'filters, and this tour — so anything that looks wrong can be pasted into an issue ' +
        'exactly as you saw it.',
      anchor: 'jump',
    },
  ],
}
