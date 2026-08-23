package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/inventory/inventorytest"
)

var t0 = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

// labelValue is the API server's label-value shape; the fake refuses
// anything else with 400, as the real server does, which is what makes a
// malformed selector value an honestly unanswerable ask.
var labelValue = regexp.MustCompile(`^([A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?)?$`)

// fakeAPI is a minimal API server: fixture objects with labels, listed
// with labelSelector equality filtering, at both the node and the
// namespaced pod collection paths.
type fakeAPI struct {
	nodes []map[string]string            // label sets
	pods  map[string][]map[string]string // namespace → label sets

	lastPath string
	status   int // non-zero forces this status on every request
}

func (f *fakeAPI) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.lastPath = r.URL.Path
		if f.status != 0 {
			http.Error(w, `{"kind":"Status","message":"forced by the fixture"}`, f.status)
			return
		}

		var sets []map[string]string
		switch {
		case r.URL.Path == "/api/v1/nodes":
			sets = f.nodes
		case r.URL.Path == "/api/v1/pods":
			for _, ns := range f.pods {
				sets = append(sets, ns...)
			}
		case strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/") && strings.HasSuffix(r.URL.Path, "/pods"):
			ns := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/"), "/pods")
			sets = f.pods[ns]
		default:
			http.NotFound(w, r)
			return
		}

		want := map[string]string{}
		if sel := r.URL.Query().Get("labelSelector"); sel != "" {
			for _, pair := range strings.Split(sel, ",") {
				k, v, ok := strings.Cut(pair, "=")
				if !ok || !labelValue.MatchString(v) {
					http.Error(w, `{"kind":"Status","message":"unable to parse requirement"}`, http.StatusBadRequest)
					return
				}
				want[k] = v
			}
		}

		var items []any
		for i, labels := range sets {
			matched := true
			for k, v := range want {
				if labels[k] != v {
					matched = false
					break
				}
			}
			if matched {
				items = append(items, map[string]any{"metadata": map[string]any{"name": fmt.Sprintf("item-%d", i), "labels": labels}})
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
}

func fixture() *fakeAPI {
	return &fakeAPI{
		nodes: []map[string]string{
			{"kubernetes.io/os": "linux", "telecraft.tier": "edge"},
			{"kubernetes.io/os": "linux", "telecraft.tier": "edge"},
			{"kubernetes.io/os": "linux", "telecraft.tier": "gateway"},
		},
		pods: map[string][]map[string]string{
			"collectors": {
				{"app": "gateway"},
				{"app": "gateway"},
			},
			"elsewhere": {
				{"app": "gateway"},
			},
		},
	}
}

func provider(t *testing.T, srv *httptest.Server, cfg KubernetesConfig) *Kubernetes {
	t.Helper()
	cfg.Endpoint = srv.URL
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return t0 }
	}
	p, err := NewKubernetes(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// The shipped conformance kit, run against the Kubernetes implementation
// over a controlled substrate, the ADR-0036 §4 discipline ADR-0035
// extends to this seam.
func TestKubernetesPassesTheKit(t *testing.T) {
	srv := httptest.NewServer(fixture().handler())
	defer srv.Close()

	inventorytest.Run(t, inventorytest.Kit{
		Provider: provider(t, srv, KubernetesConfig{}),
		Seeded: []inventorytest.Seed{
			{Selector: map[string]string{"telecraft.tier": "edge"}, Instances: 2},
			{Selector: map[string]string{"kubernetes.io/os": "linux"}, Instances: 3},
			{Selector: map[string]string{"telecraft.tier": "nothing"}, Instances: 0},
		},
		// A value the API server's label grammar refuses: the ask is
		// honestly unanswerable, and must come back Known false rather
		// than as a guessed count.
		Unanswerable: map[string]string{"telecraft.tier": "not a label value"},
	})
}

// The derived count tracks the substrate: the same ask after a scale-up
// answers the new population: the expectation floats with the autoscaler
// by construction (ADR-0035 §1).
func TestKubernetesTracksTheAutoscaler(t *testing.T) {
	api := fixture()
	srv := httptest.NewServer(api.handler())
	defer srv.Close()
	p := provider(t, srv, KubernetesConfig{})
	sel := map[string]string{"telecraft.tier": "edge"}

	if c := p.Expected(context.Background(), sel); !c.Known || c.Instances != 2 {
		t.Fatalf("before scale-up: %+v, want Known 2", c)
	}
	api.nodes = append(api.nodes, map[string]string{"kubernetes.io/os": "linux", "telecraft.tier": "edge"})
	if c := p.Expected(context.Background(), sel); !c.Known || c.Instances != 3 {
		t.Fatalf("after scale-up: %+v, want Known 3: the count is live, never cached", c)
	}
}

// Identity attributes that are not label keys verbatim map through the
// configured label map; unmapped attributes pass unchanged.
func TestKubernetesLabelMapping(t *testing.T) {
	srv := httptest.NewServer(fixture().handler())
	defer srv.Close()
	p := provider(t, srv, KubernetesConfig{
		Labels: map[string]string{"telecraft.tier.name": "telecraft.tier"},
	})

	c := p.Expected(context.Background(), map[string]string{
		"telecraft.tier.name": "edge",
		"kubernetes.io/os":    "linux",
	})
	if !c.Known || c.Instances != 2 {
		t.Fatalf("mapped selector: %+v, want Known 2", c)
	}
}

// ModePods counts pods, namespaced when configured, cluster-wide when not.
func TestKubernetesPodMode(t *testing.T) {
	api := fixture()
	srv := httptest.NewServer(api.handler())
	defer srv.Close()
	sel := map[string]string{"app": "gateway"}

	scoped := provider(t, srv, KubernetesConfig{Mode: ModePods, Namespace: "collectors"})
	if c := scoped.Expected(context.Background(), sel); !c.Known || c.Instances != 2 {
		t.Fatalf("namespaced pod count: %+v, want Known 2", c)
	}
	if api.lastPath != "/api/v1/namespaces/collectors/pods" {
		t.Fatalf("namespaced list hit %q", api.lastPath)
	}

	wide := provider(t, srv, KubernetesConfig{Mode: ModePods})
	if c := wide.Expected(context.Background(), sel); !c.Known || c.Instances != 3 {
		t.Fatalf("cluster-wide pod count: %+v, want Known 3", c)
	}
}

// A refused request is Known false with the server's answer in the cause,
// never an error, never a zero pretending to be a reading (ADR-0008).
func TestKubernetesRefusedRequestIsUnknown(t *testing.T) {
	api := fixture()
	api.status = http.StatusForbidden
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	c := provider(t, srv, KubernetesConfig{}).Expected(context.Background(), map[string]string{"telecraft.tier": "edge"})
	switch {
	case c.Known:
		t.Fatalf("a 403 became a count: %+v", c)
	case !strings.Contains(c.Cause, "403"):
		t.Fatalf("cause %q does not carry the server's status", c.Cause)
	case c.AsOf.IsZero():
		t.Fatal("the unknown count carries no as_of")
	}
}

// An unreachable API server is Known false with the transport failure as
// the cause.
func TestKubernetesUnreachableServerIsUnknown(t *testing.T) {
	srv := httptest.NewServer(fixture().handler())
	srv.Close() // dead on arrival

	p, err := NewKubernetes(KubernetesConfig{Endpoint: srv.URL, Now: func() time.Time { return t0 }})
	if err != nil {
		t.Fatal(err)
	}
	c := p.Expected(context.Background(), map[string]string{"telecraft.tier": "edge"})
	if c.Known || c.Cause == "" {
		t.Fatalf("an unreachable server did not read as an unknown with a cause: %+v", c)
	}
}

// The bearer token rides every request when configured.
func TestKubernetesSendsTheToken(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"items":[]}`)
	}))
	defer srv.Close()

	provider(t, srv, KubernetesConfig{Token: "s3cret"}).Expected(context.Background(), map[string]string{"a": "b"})
	if got != "Bearer s3cret" {
		t.Fatalf("Authorization = %q", got)
	}
}

// Wiring that could only ever produce unknown counts is refused at
// construction, not discovered one puzzling ask at a time.
func TestNewKubernetesRefusesBadWiring(t *testing.T) {
	for name, cfg := range map[string]KubernetesConfig{
		"no endpoint":          {},
		"unknown mode":         {Endpoint: "http://127.0.0.1:8001", Mode: "deployments"},
		"namespace with nodes": {Endpoint: "http://127.0.0.1:8001", Mode: ModeNodes, Namespace: "collectors"},
	} {
		if _, err := NewKubernetes(cfg); err == nil {
			t.Errorf("%s: NewKubernetes accepted the config", name)
		}
	}
}
