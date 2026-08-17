// Spike module — deliberately nested so the root module and CI never build it.
// See VERDICT.md; production home would be internal/normalise if the spike holds.
module github.com/telecraft-dev/telecraft/docs/prototypes/normaliser-spike

go 1.26.1

require gopkg.in/yaml.v3 v3.0.1
