# A served collector: the OpAMP Supervisor, with the collector it runs.
#
# The two ship as separate images. The Supervisor's own documentation lists
# "a Collector binary that you would like to control" as a thing you bring,
# so this copies one in rather than assuming which image holds what. The
# path is what devenv/identity/*.yaml names as agent.executable.
#
# Nothing is built from source and nothing is fetched at run time: two
# pulls, then the environment starts offline (ADR-0052).

ARG OTELCOL_VERSION
ARG SUPERVISOR_VERSION

FROM otel/opentelemetry-collector-contrib:${OTELCOL_VERSION} AS agent

FROM ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-opampsupervisor:${SUPERVISOR_VERSION}
COPY --from=agent /otelcol-contrib /otelcol-contrib
