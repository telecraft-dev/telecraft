# The Telecraft image: one binary, the Catalogue baseline it was built
# against, and the licence, on a distroless base (ADR-0068 §2).
#
# It is assembled, not compiled. There is no build stage: tools/image/stage.sh
# puts the binaries under dist/image/ and this copies them in, so the bytes a
# release attaches and the bytes inside the image are one build rather than
# two of the same commit. `docker build .` on a checkout with nothing staged
# fails on a missing file, and that is the intended failure: the alternative
# is an image that quietly holds a different binary from the one whose
# checksum was published.
#
# Nothing is fetched while this builds. There is no RUN instruction, so the
# only network the build needs is the pull of the base layers (REQ-006).
#
# The base is pinned by digest, which is what keeps the image a function of
# the tag: refreshing it is a commit, which is a review. It supplies the
# three things a static Go binary cannot supply itself, a certificate bundle
# for the provider round trips, timezone data, and a non-root user, and it
# carries no package manager, no shell and no interpreter.
#
# Resolved 28 August 2026 from gcr.io/distroless/static-debian12:nonroot.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

# TARGETARCH is BuildKit's, one value per platform in the index, so the
# multi-architecture index is one pass over binaries that are already built
# (ADR-0068 §2). VERSION and REVISION are provenance the labels carry; the
# console names its own version from the value it was built with.
ARG TARGETARCH
ARG VERSION=development
ARG REVISION=

LABEL org.opencontainers.image.title="Telecraft" \
      org.opencontainers.image.description="Craft, govern, and verify OpenTelemetry across your whole estate." \
      org.opencontainers.image.url="https://telecraft.dev" \
      org.opencontainers.image.source="https://github.com/telecraft-dev/telecraft" \
      org.opencontainers.image.licenses="Elastic-2.0" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"

# The whole CLI, so a pipeline with no Go toolchain runs `telecraft check`
# from the same artefact its control plane runs.
COPY dist/image/telecraft-linux-${TARGETARCH} /usr/local/bin/telecraft

# Elastic License 2.0 asks that whoever is given part of the software is
# given the terms with it (ADR-0050), and an image is a copy handed to
# somebody.
COPY dist/image/LICENSE /usr/share/telecraft/LICENSE

# The Catalogue for the collector version this repository pins: the
# build-time baseline of ADR-0020 §5. It reaches an estate through the same
# import and activation pipeline as a Catalogue carried across an air gap,
# so an instance that never touches a network can still judge on the day it
# starts.
COPY dist/image/catalogues/ /usr/share/telecraft/catalogues/

# serve defaults to loopback, which is right on a host and means unreachable
# inside a network namespace (ADR-0068 §2). Every flag has an environment
# variable under it (ADR-0067 §4), so the image configures the process
# without an entrypoint script rewriting arguments, and a flag still wins.
# Nothing else is set: no external URL, no session key and no estate, because
# those are the operator's and two of them are secret.
ENV TELECRAFT_HTTP=0.0.0.0:4321 \
    TELECRAFT_LISTEN=0.0.0.0:4320

EXPOSE 4321 4320

# The base's non-root user, named by number because the image carries no
# passwd lookup worth relying on and nothing resolves a name at start.
USER 65532:65532

ENTRYPOINT ["/usr/local/bin/telecraft"]
CMD ["serve"]
