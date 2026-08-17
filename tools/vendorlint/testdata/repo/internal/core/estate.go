package core

type FleetProvider interface{} // seeded violation: the ADR-0001 canonical sin

const defaultBackend = "elasticsearch" // seeded violation: vendor word in core

type EstateProvider interface{} // domain term; not a finding
