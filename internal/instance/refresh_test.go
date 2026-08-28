package instance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/forge"
	"github.com/telecraft-dev/telecraft/internal/serving"
)

// fakePush is a forge that verifies a delivery the way one would: an HMAC
// over the bytes that arrived, keyed by the secret the deployment placed.
type fakePush struct{}

func (fakePush) Push(n forge.Notification, secret string) (bool, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(n.Body)
	if n.Header.Get("Signature") != hex.EncodeToString(mac.Sum(nil)) {
		return false, errors.New("the delivery is not signed with the secret this instance holds")
	}
	return n.Header.Get("Event") == "push", nil
}

func signed(secret string, body []byte, event string) *http.Request {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/refresh", strings.NewReader(string(body)))
	req.Header.Set("Signature", hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("Event", event)
	return req
}

// startRefreshable is an Instance whose deployment placed both of the
// refresh endpoint's credentials.
func startRefreshable(t *testing.T, key, secret string) (*Server, string) {
	t.Helper()
	root := estateCheckout(t)
	srv, err := New(Config{
		Source:        serving.DirSource{Root: root},
		Root:          root,
		HTTPEndpoint:  "127.0.0.1:0",
		FetchInterval: time.Hour,
		Sessions:      sessions(t),
		Logf:          t.Logf,
		RefreshKey:    func() (string, error) { return key, nil },
		PushSecret:    func() (string, error) { return secret, nil },
		Notifications: fakePush{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})
	return srv, "http://" + srv.HTTPAddr()
}

func post(t *testing.T, base string, req *http.Request, bearer string) (string, int) {
	t.Helper()
	url := base + "/api/v1/refresh"
	if req == nil {
		var err error
		req, err = http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
	} else {
		parsed, err := http.NewRequest(req.Method, url, req.Body)
		if err != nil {
			t.Fatal(err)
		}
		parsed.Header = req.Header
		req = parsed
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return string(body), res.StatusCode
}

// The refresh endpoint has two callers and no session: a delivery the
// forge signed, and a bare request carrying the key the deployment placed.
// Both mean the same thing, which is fetch now.
func TestARefreshHasTwoCallersAndNeitherIsSignedIn(t *testing.T) {
	const key, secret = "the refresh key", "the push secret"
	_, base := startRefreshable(t, key, secret)

	body, status := post(t, base, nil, key)
	if status != http.StatusAccepted || !strings.Contains(body, "fetching") {
		t.Errorf("a bare request with the key = %d %q, want 202 and what it did", status, body)
	}

	body, status = post(t, base, signed(secret, []byte(`{"ref":"refs/heads/main"}`), "push"), "")
	if status != http.StatusAccepted || !strings.Contains(body, "fetching") {
		t.Errorf("a signed push = %d %q, want 202 and what it did", status, body)
	}

	// Genuine, and not a push. Nothing to do, and nothing is wrong.
	body, status = post(t, base, signed(secret, []byte(`{}`), "installation"), "")
	if status != http.StatusAccepted || !strings.Contains(body, "nothing to fetch") {
		t.Errorf("a signed delivery that is not a push = %d %q, want 202 and nothing done", status, body)
	}
}

// A request carrying neither credential is not accepted, and the refusal
// says so without saying which check it failed.
func TestARefreshNobodyProvedIsRefused(t *testing.T) {
	const key, secret = "the refresh key", "the push secret"
	_, base := startRefreshable(t, key, secret)

	for name, tc := range map[string]struct {
		req    *http.Request
		bearer string
	}{
		"nothing at all":                     {},
		"the wrong key":                      {bearer: "not the key"},
		"a delivery signed by somebody else": {req: signed("another secret", []byte(`{}`), "push")},
	} {
		t.Run(name, func(t *testing.T) {
			body, status := post(t, base, tc.req, tc.bearer)
			if status != http.StatusUnauthorized {
				t.Errorf("= %d %q, want 401", status, body)
			}
			if !strings.Contains(body, refreshNotAccepted) {
				t.Errorf("the refusal is %q, want the one sentence it says", body)
			}
		})
	}
}

// A deployment that placed neither credential takes no refresh request,
// and says that rather than refusing what somebody presented: an absence
// declares the capability unavailable.
func TestAnInstanceGivenNeitherCredentialTakesNoRefresh(t *testing.T) {
	_, base := start(t, estateCheckout(t))
	body, status := post(t, base, nil, "anything")
	if status != http.StatusNotImplemented {
		t.Errorf("= %d %q, want 501", status, body)
	}
	if !strings.Contains(body, refreshUnavailable) {
		t.Errorf("the answer is %q, want the sentence that says none was placed", body)
	}
}

// The endpoint is a shortcut and never a dependency: what it does is ask
// the poll to run, and a burst of asking is one fetch rather than a queue
// of them. Nothing durable is added by any of it.
func TestABurstOfRefreshesIsOneFetch(t *testing.T) {
	srv, base := startRefreshable(t, "the refresh key", "the push secret")
	for range 20 {
		if _, status := post(t, base, nil, "the refresh key"); status != http.StatusAccepted {
			t.Fatalf("a refresh was not accepted: %d", status)
		}
	}
	if held := len(srv.nudge); held > 1 {
		t.Errorf("the server holds %d asks, want at most the one slot it has", held)
	}
}
