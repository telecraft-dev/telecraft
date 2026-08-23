import type { Tour } from './types'

/**
 * The welcome Tour: the one every reader needs, and the only one offered
 * from the chrome. It opens once per user on a bare landing URL (ADR-0051
 * §7) and can be taken again whenever.
 *
 * It teaches *this console*: where things are, and which of them are
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
  summary: 'A walk through the estate you are signed in to. It changes nothing.',
  steps: [
    {
      id: 'welcome',
      title: 'Welcome to Telecraft',
      body:
        'Telecraft builds collector configuration from governed building blocks. It then ' +
        'works out what telemetry that configuration should produce, and checks that it ' +
        'arrived. Green means the configuration worked, not only that it applied. This tour ' +
        'runs over your own estate and changes nothing. Press Escape to leave at any point.',
      demoBody:
        'You are looking at the real console over a public demo estate. It is rebuilt from ' +
        'git on every push and is read-only. Telecraft builds collector configuration from ' +
        'governed building blocks, then checks that the telemetry that configuration ' +
        'implies actually arrived. This tour changes nothing. Press Escape to leave at any ' +
        'point.',
      to: '/estate',
    },
    {
      id: 'shelf',
      title: "The shelf answers 'how are we doing?'",
      body:
        'Each card is a Tier: one position in your collection topology, with its own ' +
        'rendered configuration. The cards with the worst problems come first. You start ' +
        "on your own team's Tiers, and one click shows the whole estate. Healthy cards " +
        'stay on screen, so a missing card never passes for a healthy one.',
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
        'telemetry is right. These are three different failures with three different ' +
        'owners, so each band keeps its own mark and its own word. Colour only reinforces ' +
        'them.',
      anchor: 'card-bands',
      to: '/estate',
      search: { view: 'shelf' },
    },
    {
      id: 'lens',
      title: 'The environment lens',
      body:
        'Production leads by default. The lens sets the emphasis and the evaluation ' +
        'context. It is never a filter, so a staging problem cannot vanish because you were ' +
        'looking at production. Explicit filters live on the flat list, along with ' +
        'per-collector detail.',
      anchor: 'lens',
    },
    {
      id: 'topology',
      title: 'How telemetry flows',
      body:
        "Tiers, the hops between them, and each Service's path through the graph. " +
        'Collectors are never drawn: each one matches a Tier by selector and appears in ' +
        "that Tier's count. Telecraft derives the edges from the configuration rather than " +
        'arranging them by hand, so the picture always agrees with what is rendered.',
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
        'decides, so git stays the source of truth.',
      anchor: 'compose',
      to: '/compose',
    },
    {
      id: 'catalogue',
      title: 'What you may use, and how to ask for more',
      body:
        'The catalogue lists what exists at a given collector version. Your effective ' +
        'palette is that list, narrowed by the allow-list your team inherits, and every ' +
        'entry says why it is there. To use something outside it, ask for a Grant. A Grant ' +
        'is another pull request.',
      anchor: 'catalogue',
      to: '/catalogue',
      search: { view: 'browse' },
    },
    {
      id: 'urls',
      title: 'Everything is one URL away',
      body:
        'Jump to any Tier, Service, Blueprint, or Component from here, or press Command-K. ' +
        'Every state you can reach is in the address bar: the view, the selection, the ' +
        'filters, and this tour. Paste the URL into an issue and a colleague sees exactly ' +
        'what you saw.',
      anchor: 'jump',
    },
  ],
}
