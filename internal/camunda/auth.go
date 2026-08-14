package camunda

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// BasicAuthTransport returns a RoundTripper that attaches HTTP Basic
// credentials (Camunda 8.8+ Self-Managed basic auth, e.g. c8run demo/demo).
func BasicAuthTransport(username, password string) http.RoundTripper {
	return &basicAuthTransport{base: http.DefaultTransport, username: username, password: password}
}

type basicAuthTransport struct {
	base     http.RoundTripper
	username string
	password string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.SetBasicAuth(t.username, t.password)
	return t.base.RoundTrip(r)
}

// OAuthTransport returns a RoundTripper that obtains OAuth2
// client-credentials tokens (form-encoded, with Camunda's non-standard
// audience parameter), caches them in memory, and retries a request once
// on 401 with a freshly fetched token.
func OAuthTransport(tokenURL, clientID, clientSecret, audience, scope string) http.RoundTripper {
	return &oauthTransport{
		base:         http.DefaultTransport,
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		audience:     audience,
		scope:        scope,
		// The token endpoint gets its own plain client: routing it through
		// this transport would recurse (and deadlock on the token mutex).
		tokenClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type oauthTransport struct {
	base         http.RoundTripper
	tokenURL     string
	clientID     string
	clientSecret string
	audience     string
	scope        string
	tokenClient  *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

func (t *oauthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, fresh, err := t.getToken()
	if err != nil {
		return nil, err
	}
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+token)
	resp, err := t.base.RoundTrip(r)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || fresh {
		// A freshly fetched token can't get fresher — return the 401 as-is.
		return resp, err
	}
	// Bodies are consumed after the first send; without GetBody a replay
	// would send an empty body, so return the 401 rather than retry wrong.
	if req.Body != nil && req.GetBody == nil {
		return resp, nil
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	t.invalidate(token)
	token, _, err = t.getToken()
	if err != nil {
		return nil, err
	}
	r2 := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		r2.Body = body
	}
	r2.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(r2)
}

// getToken returns a valid token and whether it was fetched just now.
// The mutex is held across the fetch (double-checked) so concurrent
// callers coalesce into one token request; the fetch itself is bounded
// by tokenClient's timeout.
func (t *oauthTransport) getToken() (string, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && time.Now().Before(t.expiry) {
		return t.token, false, nil
	}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {t.clientID},
		"client_secret": {t.clientSecret},
	}
	if t.audience != "" {
		form.Set("audience", t.audience)
	}
	if t.scope != "" {
		form.Set("scope", t.scope)
	}
	// Detached context: the fetch serves all waiting requests, so it must
	// not die with whichever request happened to trigger it.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.tokenClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("oauth token fetch from %s: %w", t.tokenURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return "", false, fmt.Errorf("oauth: %s from %s", oauthErrorSummary(body, resp.Status), t.tokenURL)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", false, fmt.Errorf("oauth: invalid token response from %s: %v", t.tokenURL, err)
	}
	if tok.AccessToken == "" {
		return "", false, fmt.Errorf("oauth: empty access_token from %s", t.tokenURL)
	}
	// RFC 6749 allows omitting expires_in; floor tiny/missing lifetimes so
	// we don't fetch per request, and refresh a bit before real expiry.
	lifetime := time.Duration(tok.ExpiresIn) * time.Second
	if lifetime <= 30*time.Second {
		lifetime = 30 * time.Second
	}
	margin := min(30*time.Second, lifetime/2)
	t.token = tok.AccessToken
	t.expiry = time.Now().Add(lifetime - margin)
	return t.token, true, nil
}

// invalidate clears the cached token only if it is still the one that
// failed — a concurrent refresh may already have replaced it.
func (t *oauthTransport) invalidate(failed string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token == failed {
		t.token = ""
	}
}

func oauthErrorSummary(body []byte, status string) string {
	var e struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		if e.Description != "" {
			return fmt.Sprintf("%s (%s)", e.Error, truncateStr(e.Description, 120))
		}
		return e.Error
	}
	if len(body) > 0 {
		return fmt.Sprintf("%s: %s", status, truncateStr(string(body), 120))
	}
	return status
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
