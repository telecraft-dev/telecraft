#!/usr/bin/env bash
# One-command live verification for the Elasticsearch TelemetryProvider
# (issue #10, criterion 1). From the repo root:
#
#   internal/provider/telemetry/demo.sh
#
# It starts a throwaway single-node Elasticsearch container, seeds a fixture
# Service ("checkout": logs present, metrics index empty, traces index never
# created), runs the CLI against it — including once against an unreachable
# endpoint to show the degraded readings (criterion 2) — and finishes with
# the gated live test suite. The container is removed on exit.
#
# Needs: docker, curl, go. Nothing here runs in CI's default test job; plain
# `go test ./...` stays green without Docker because the live tests skip.
set -euo pipefail
cd "$(dirname "$0")/../../.."

IMAGE="${TELECRAFT_DEMO_IMAGE:-docker.elastic.co/elasticsearch/elasticsearch:9.1.0}"
NAME=telecraft-demo-es
PORT="${TELECRAFT_DEMO_PORT:-9200}"
ENDPOINT="http://localhost:${PORT}"

docker rm -f "$NAME" >/dev/null 2>&1 || true
echo ">> starting $IMAGE on :$PORT"
docker run -d --name "$NAME" -p "127.0.0.1:${PORT}:9200" \
  -e discovery.type=single-node \
  -e xpack.security.enabled=false \
  -e ES_JAVA_OPTS="-Xms512m -Xmx512m" \
  "$IMAGE" >/dev/null
trap 'docker rm -f "$NAME" >/dev/null' EXIT

echo ">> waiting for the cluster"
for _ in $(seq 1 60); do
  curl -fsS "$ENDPOINT/_cluster/health" >/dev/null 2>&1 && break
  sleep 2
done
curl -fsS "$ENDPOINT/_cluster/health" >/dev/null

echo ">> seeding the fixture Service (checkout): logs only — no metrics or traces index"
NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
curl -fsS -XDELETE "$ENDPOINT/logs-demo?ignore_unavailable=true" >/dev/null
curl -fsS -XPUT "$ENDPOINT/logs-demo" -H 'Content-Type: application/json' -d '{
  "mappings": {
    "dynamic_templates": [
      {"strings_as_keyword": {"match_mapping_type": "string", "mapping": {"type": "keyword"}}}
    ],
    "properties": {"@timestamp": {"type": "date"}}
  }
}' >/dev/null
curl -fsS -XPOST "$ENDPOINT/logs-demo/_bulk?refresh=true" -H 'Content-Type: application/x-ndjson' --data-binary @- >/dev/null <<BULK
{"index":{}}
{"@timestamp": "$NOW", "resource": {"attributes": {"service.name": "checkout"}}, "attributes": {"url.path": "/pay", "http.request.method": "POST"}}
{"index":{}}
{"@timestamp": "$NOW", "resource": {"attributes": {"service.name": "checkout"}}, "attributes": {"url.path": "/pay", "http.request.method": "GET"}}
{"index":{}}
{"@timestamp": "$NOW", "resource": {"attributes": {"service.name": "checkout"}}, "attributes": {"url.path": "/health"}}
{"index":{}}
{"@timestamp": "$NOW", "resource": {"attributes": {"service.name": "somebody-else"}}, "attributes": {"noise": "y"}}
BULK

echo
echo ">> CLI against the live backend (logs land; metrics-*/traces-* match no index -> Known false)"
go run ./cmd/telecraft observe -endpoint "$ENDPOINT" -service checkout -window 15m \
  -attributes resource.attributes.service.name,attributes.http.request.method

echo
echo ">> CLI against an unreachable backend (criterion 2: degraded, never a crash)"
go run ./cmd/telecraft observe -endpoint "http://127.0.0.1:1" -service checkout -window 15m -timeout 5s

echo
echo ">> gated live test suite"
TELECRAFT_TELEMETRY_LIVE_ENDPOINT="$ENDPOINT" go test ./internal/provider/telemetry/ -run Live -v -count=1

echo
echo ">> demo complete"
