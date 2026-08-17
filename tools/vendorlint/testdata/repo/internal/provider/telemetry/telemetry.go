package telemetry

type Fleet struct{} // seeded violation: unqualified vendor product name

type ElasticFleet struct{} // qualified per ADR-0001; not a finding

type Elasticsearch struct{} // qualified product name; not a finding
