package core

type FleetProvider interface{} // seeded violation: the ADR-0001 canonical sin

const defaultBackend = "elasticsearch" // seeded violation: vendor word in core

type EstateProvider interface{} // domain term; not a finding

type GitHubClient struct{} // seeded violation: forge type in core (ADR-0028 §4)

const modulePath = "github.com/telecraft-dev/telecraft" // module namespace; not a finding
