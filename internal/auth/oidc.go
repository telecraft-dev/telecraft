package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OIDC is the first-party redirect provider (ADR-0019 §1): the
// authorization-code flow against any OpenID Connect issuer, self-hosted
// identity providers in air gaps included. The implementation is standard
// library only — discovery, code exchange and RS256 ID-token verification
// — so the air-gap binary needs nothing beyond reach of its own issuer
// (REQ-006).
type OIDC struct {
	// Issuer is the issuer URL, exactly as the provider names itself —
	// discovery is served beneath it, and every ID token must carry it.
	Issuer string

	// ClientID and ClientSecret identify this instance to the issuer.
	ClientID     string
	ClientSecret string

	// HTTPClient is the client discovery, token exchange and JWKS fetches
	// go through. Nil means a 10-second-timeout default.
	HTTPClient *http.Client

	mu   sync.Mutex
	disc *oidcDiscovery
}

// Name implements Provider.
func (*OIDC) Name() string { return "oidc" }

// oidcDiscovery is the slice of the provider metadata this flow needs.
type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// Begin implements RedirectProvider: the authorization request URL. The
// nonce and the PKCE verifier are both derived from the caller's state, so
// replay protection and code binding need no server-side storage. Complete
// recomputes both, requires the ID token to carry the nonce, and presents
// the verifier the challenge here commits to.
func (o *OIDC) Begin(ctx context.Context, state, callbackURL string) (string, error) {
	disc, err := o.discover(ctx)
	if err != nil {
		return "", err
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {o.ClientID},
		"redirect_uri":          {callbackURL},
		"scope":                 {"openid profile email"},
		"state":                 {state},
		"nonce":                 {nonceFrom(state)},
		"code_challenge":        {challengeFor(verifierFrom(state))},
		"code_challenge_method": {"S256"},
	}
	sep := "?"
	if strings.Contains(disc.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	return disc.AuthorizationEndpoint + sep + q.Encode(), nil
}

// Complete implements RedirectProvider: exchange the code, verify the ID
// token, return the claims as an Identity.
func (o *OIDC) Complete(ctx context.Context, state, callbackURL string, params url.Values) (Identity, error) {
	if e := params.Get("error"); e != "" {
		return Identity{}, fmt.Errorf("the identity provider refused the sign-in: %s %s", e, params.Get("error_description"))
	}
	code := params.Get("code")
	if code == "" {
		return Identity{}, fmt.Errorf("the callback carries no authorization code")
	}

	disc, err := o.discover(ctx)
	if err != nil {
		return Identity{}, err
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {callbackURL},
		"client_id":     {o.ClientID},
		"client_secret": {o.ClientSecret},
		"code_verifier": {verifierFrom(state)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, disc.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := o.client().Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("token exchange: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return Identity{}, fmt.Errorf("token exchange: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("token exchange: %s from the token endpoint", res.Status)
	}
	var token struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return Identity{}, fmt.Errorf("token exchange: %w", err)
	}
	if token.IDToken == "" {
		return Identity{}, fmt.Errorf("token exchange: the response carries no id_token")
	}

	claims, err := o.verifyIDToken(ctx, disc, token.IDToken, nonceFrom(state))
	if err != nil {
		return Identity{}, err
	}
	id := Identity{Subject: claims.Subject, Name: claims.Name, Email: claims.Email}
	if id.Name == "" {
		id.Name = claims.PreferredUsername
	}
	if err := id.valid(); err != nil {
		return Identity{}, fmt.Errorf("the ID token carries no usable subject and email claims — attribution needs both (ADR-0019 §3)")
	}
	return id, nil
}

// idClaims is the slice of the ID token this flow reads.
type idClaims struct {
	Issuer            string   `json:"iss"`
	Subject           string   `json:"sub"`
	Audience          audience `json:"aud"`
	Expires           int64    `json:"exp"`
	IssuedAt          int64    `json:"iat"`
	NotBefore         int64    `json:"nbf"`
	Nonce             string   `json:"nonce"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
}

// clockSkew is how far this instance's clock may disagree with the
// issuer's before a sound ID token is refused. Thirty seconds is the size
// the two ends of that comparison argue for. It is wider than the drift a
// host keeps under normal time discipline, and wide enough for an
// air-gapped deployment whose issuer is disciplined by a local time source
// rather than a public pool (ADR-0019), where a sign-in must not start
// failing because the two clocks last agreed some hours ago. It is also
// well under a minute, so a token whose expiry passed a minute ago is
// still refused and the allowance never meaningfully lengthens the window
// a stolen token is replayable in. Both directions matter: the allowance
// applies to exp, and to iat and nbf on the other side, where a clock that
// runs fast at the issuer is what makes a valid token look premature.
const clockSkew = 30 * time.Second

// audience decodes the aud claim, which is one string or a list of them.
type audience []string

func (a *audience) UnmarshalJSON(raw []byte) error {
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return err
	}
	*a = audience(many)
	return nil
}

// verifyIDToken checks signature (RS256 against the issuer's JWKS), issuer,
// audience, the time claims and nonce. Anything else the token carries is
// ignored.
func (o *OIDC) verifyIDToken(ctx context.Context, disc *oidcDiscovery, raw, wantNonce string) (idClaims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return idClaims{}, fmt.Errorf("id_token is not a three-part JWT")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return idClaims{}, fmt.Errorf("id_token header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return idClaims{}, fmt.Errorf("id_token header: %w", err)
	}
	if header.Alg != "RS256" {
		return idClaims{}, fmt.Errorf("id_token is signed with %q — RS256 is what this provider verifies; configure the issuer's client to use it", header.Alg)
	}

	key, err := o.signingKey(ctx, disc.JWKSURI, header.Kid)
	if err != nil {
		return idClaims{}, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return idClaims{}, fmt.Errorf("id_token signature: %w", err)
	}
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return idClaims{}, fmt.Errorf("id_token signature does not verify against the issuer's keys")
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return idClaims{}, fmt.Errorf("id_token claims: %w", err)
	}
	var claims idClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return idClaims{}, fmt.Errorf("id_token claims: %w", err)
	}
	// The time claims are read against one instant, so a verification that
	// straddles a second cannot judge two of them by different clocks. iat
	// and nbf are optional in an ID token, so each is checked only when the
	// token asserts it; exp is mandatory, and a token that omits it reads
	// as expired at the epoch.
	now := time.Now()
	switch {
	case claims.Issuer != o.Issuer:
		return idClaims{}, fmt.Errorf("id_token issuer %q is not the configured issuer %q", claims.Issuer, o.Issuer)
	case !claims.Audience.contains(o.ClientID):
		return idClaims{}, fmt.Errorf("id_token audience does not include this client")
	case now.Add(-clockSkew).Unix() >= claims.Expires:
		return idClaims{}, fmt.Errorf("id_token has expired")
	case claims.NotBefore != 0 && now.Add(clockSkew).Unix() < claims.NotBefore:
		return idClaims{}, fmt.Errorf("id_token is not valid yet: its nbf claim is further ahead than the clock-skew allowance covers")
	case claims.IssuedAt != 0 && now.Add(clockSkew).Unix() < claims.IssuedAt:
		return idClaims{}, fmt.Errorf("id_token is issued in the future: its iat claim is further ahead than the clock-skew allowance covers")
	case claims.Nonce != wantNonce:
		return idClaims{}, fmt.Errorf("id_token nonce does not match this sign-in attempt")
	}
	return claims, nil
}

func (a audience) contains(clientID string) bool {
	for _, aud := range a {
		if aud == clientID {
			return true
		}
	}
	return false
}

// signingKey fetches the issuer's JWKS and returns the RSA key the token
// names. Fetched per verification, uncached: sign-ins are rare and key
// rotation must never strand them.
func (o *OIDC) signingKey(ctx context.Context, jwksURI, kid string) (*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, err
	}
	res, err := o.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("JWKS fetch: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS fetch: %s", res.Status)
	}
	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("JWKS fetch: %w", err)
	}
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || (kid != "" && k.Kid != kid) {
			continue
		}
		n, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, fmt.Errorf("JWKS key %q: %w", k.Kid, err)
		}
		e, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, fmt.Errorf("JWKS key %q: %w", k.Kid, err)
		}
		return &rsa.PublicKey{
			N: new(big.Int).SetBytes(n),
			E: int(new(big.Int).SetBytes(e).Int64()),
		}, nil
	}
	return nil, fmt.Errorf("the issuer's JWKS holds no RSA key %q", kid)
}

// discover fetches and caches the provider metadata. Cached for the
// process: the endpoints of an issuer do not move under a running
// instance, and the air-gap instance should not chatter.
func (o *OIDC) discover(ctx context.Context) (*oidcDiscovery, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.disc != nil {
		return o.disc, nil
	}
	if o.Issuer == "" || o.ClientID == "" {
		return nil, fmt.Errorf("the oidc provider needs an issuer and a client id")
	}
	well := strings.TrimSuffix(o.Issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, well, nil)
	if err != nil {
		return nil, err
	}
	res, err := o.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("issuer discovery: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("issuer discovery: %s from %s", res.Status, well)
	}
	var disc oidcDiscovery
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&disc); err != nil {
		return nil, fmt.Errorf("issuer discovery: %w", err)
	}
	if disc.AuthorizationEndpoint == "" || disc.TokenEndpoint == "" || disc.JWKSURI == "" {
		return nil, fmt.Errorf("issuer discovery: %s does not name the authorization, token and JWKS endpoints", well)
	}
	o.disc = &disc
	return o.disc, nil
}

func (o *OIDC) client() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// nonceFrom binds the OIDC nonce to the caller's CSRF state, so replay
// protection is stateless end to end.
func nonceFrom(state string) string {
	sum := sha256.Sum256([]byte("telecraft-oidc-nonce." + state))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// verifierFrom binds the PKCE code verifier to the caller's CSRF state the
// same way nonceFrom binds the nonce, so the authorization code is bound to
// this sign-in attempt without a server-side store. The state is already a
// high-entropy secret held only in the caller's signed cookie, and the
// label keeps this derivation separate from the nonce's, so neither value
// discloses the other. The result is 43 unreserved characters, which is
// what RFC 7636 asks a verifier to be.
//
// The instance holds a client secret, so PKCE here is defence in depth
// rather than the only thing binding the code. It is what OAuth 2.1
// requires, and it costs no coordination, which is what keeps ADR-0019's
// air-gapped deployment free of shared state.
func verifierFrom(state string) string {
	sum := sha256.Sum256([]byte("telecraft-oidc-verifier." + state))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// challengeFor is the S256 transformation RFC 7636 defines: the base64url
// of the SHA-256 of the verifier's ASCII bytes. Only S256 is offered,
// because "plain" leaves the code bound to a value the authorization
// request already put in front of the browser.
func challengeFor(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
