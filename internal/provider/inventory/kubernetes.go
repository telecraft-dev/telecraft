// Package inventory holds the InventoryProvider implementations
// (ADR-0035).
//
// The first is Kubernetes: the substrate answering live from its own API
// (ADR-0012) — how many nodes match this Tier's workload selector right
// now — so the expectation floats with the autoscaler by construction. A
// scale-up is a bigger floor on the next ask, never a stale declaration
// to chase. The provider needs API access scoped to node and workload
// reads; that is deployment documentation, never model surface (ADR-0035).
package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	seam "github.com/telecraft-dev/telecraft/internal/inventory"
)

// DefaultRefreshCadence is the declared cadence when none is configured.
// Every Expected call reads the API live, so the cadence describes how
// often the platform is expected to re-ask — the input the staleness
// arithmetic needs (ADR-0036 §3), not a cache lifetime.
const DefaultRefreshCadence = time.Minute

// Mode names what the provider counts.
type Mode string

const (
	// ModeNodes counts nodes matching the selector — the DaemonSet-shaped
	// default: one collector per matching node, so the node count is the
	// expected population (ADR-0035 §1).
	ModeNodes Mode = "nodes"

	// ModePods counts pods matching the selector — for Deployment-shaped
	// Tiers where the workload's own pod labels carry the identity.
	ModePods Mode = "pods"
)

// KubernetesConfig configures one Kubernetes provider.
type KubernetesConfig struct {
	// Endpoint is the API server base URL — in-cluster
	// "https://kubernetes.default.svc", or a local "kubectl proxy" at
	// "http://127.0.0.1:8001". Mandatory.
	Endpoint string

	// Token is the bearer token sent on every request; empty sends none
	// (the proxy case).
	Token string

	// Client is the HTTP client used; nil means http.DefaultClient. TLS
	// configuration (the cluster CA) belongs on the client.
	Client *http.Client

	// Mode is what the provider counts; empty means ModeNodes.
	Mode Mode

	// Namespace narrows ModePods to one namespace; empty means all. Never
	// set with ModeNodes — nodes are not namespaced.
	Namespace string

	// Labels maps identity attribute names to node or pod label keys, for
	// estates whose selector attributes are not label keys verbatim. An
	// attribute absent from the map is used as the label key unchanged.
	Labels map[string]string

	// RefreshCadence is the cadence the provider declares (ADR-0036 §3);
	// zero means DefaultRefreshCadence.
	RefreshCadence time.Duration

	// Now is the clock counts are stamped with; nil means time.Now.
	Now func() time.Time
}

// Kubernetes is the Kubernetes InventoryProvider (ADR-0035): it answers
// the seam's one question from the API server, live, holding no state
// between asks.
type Kubernetes struct {
	endpoint  string
	token     string
	client    *http.Client
	mode      Mode
	namespace string
	labels    map[string]string
	cadence   time.Duration
	now       func() time.Time
}

var _ seam.Provider = (*Kubernetes)(nil)

// NewKubernetes builds the provider. Configuration that could only ever
// produce unknown counts — no endpoint, an unknown mode, a namespace on a
// node count — is refused here, at wiring time, rather than surfacing as
// a puzzling Known false on every ask.
func NewKubernetes(cfg KubernetesConfig) (*Kubernetes, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("the Kubernetes provider needs an API server endpoint")
	}
	if _, err := url.Parse(cfg.Endpoint); err != nil {
		return nil, fmt.Errorf("endpoint %q is not a URL: %w", cfg.Endpoint, err)
	}
	mode := cfg.Mode
	if mode == "" {
		mode = ModeNodes
	}
	if mode != ModeNodes && mode != ModePods {
		return nil, fmt.Errorf("mode %q is not one of nodes, pods", cfg.Mode)
	}
	if mode == ModeNodes && cfg.Namespace != "" {
		return nil, fmt.Errorf("a namespace narrows a pod count — nodes are not namespaced")
	}
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	cadence := cfg.RefreshCadence
	if cadence <= 0 {
		cadence = DefaultRefreshCadence
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Kubernetes{
		endpoint:  strings.TrimRight(cfg.Endpoint, "/"),
		token:     cfg.Token,
		client:    client,
		mode:      mode,
		namespace: cfg.Namespace,
		labels:    cfg.Labels,
		cadence:   cadence,
		now:       now,
	}, nil
}

// Name identifies the implementation as runtime data (ADR-0001).
func (p *Kubernetes) Name() string { return "kubernetes" }

// Declaration is the static contract declaration (ADR-0036 §3).
func (p *Kubernetes) Declaration() seam.Declaration {
	return seam.Declaration{RefreshCadence: p.cadence}
}

// Expected asks the API server how many nodes (or pods) match the
// selector right now. Every failure to answer — no selector, an
// unreachable server, a refused request, an unreadable response — is
// Known false with a cause, never an error and never a guess (ADR-0008).
// A selector matching nothing is a count of zero: the substrate honestly
// expecting nothing.
func (p *Kubernetes) Expected(ctx context.Context, selector map[string]string) seam.Count {
	asOf := p.now()
	unknown := func(cause string) seam.Count {
		return seam.Count{Known: false, Cause: cause, AsOf: asOf}
	}

	if len(selector) == 0 {
		return unknown("the Tier has no selector — a selector-less Tier matches nothing (ADR-0030), and counting everything would answer a question nobody asked")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.listURL(selector), nil)
	if err != nil {
		return unknown(fmt.Sprintf("the list request could not be built: %v", err))
	}
	req.Header.Set("Accept", "application/json")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return unknown(fmt.Sprintf("the API server did not answer: %v", err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return unknown(fmt.Sprintf("the API server's response could not be read: %v", err))
	}
	if resp.StatusCode != http.StatusOK {
		return unknown(fmt.Sprintf("the API server answered %s: %s — a refused or failed list never becomes a count", resp.Status, snippet(body)))
	}

	var list struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return unknown(fmt.Sprintf("the API server's response was not a list: %v — a response we cannot read must become Known false, never a guess", err))
	}
	return seam.Count{Known: true, AsOf: asOf, Instances: len(list.Items)}
}

// listURL builds the list request: the mode's collection path plus the
// selector translated to a label selector, attribute keys mapped through
// the configured label map and passed verbatim otherwise, sorted for a
// stable URL.
func (p *Kubernetes) listURL(selector map[string]string) string {
	attrs := make([]string, 0, len(selector))
	for k := range selector {
		attrs = append(attrs, k)
	}
	sort.Strings(attrs)
	pairs := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		label := attr
		if mapped, ok := p.labels[attr]; ok {
			label = mapped
		}
		pairs = append(pairs, label+"="+selector[attr])
	}

	path := "/api/v1/nodes"
	if p.mode == ModePods {
		path = "/api/v1/pods"
		if p.namespace != "" {
			path = "/api/v1/namespaces/" + url.PathEscape(p.namespace) + "/pods"
		}
	}
	return p.endpoint + path + "?labelSelector=" + url.QueryEscape(strings.Join(pairs, ","))
}

// snippet keeps an error body readable in a one-line cause.
func snippet(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		return "(empty body)"
	}
	return s
}
