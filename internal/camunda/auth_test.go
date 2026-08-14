package camunda

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const topologyJSON = `{"brokers":[],"clusterId":"t","clusterSize":1,"partitionsCount":1,"replicationFactor":1,"gatewayVersion":"8.10.0"}`
const emptySearchJSON = `{"items":[],"page":{"totalItems":0}}`

func TestBasicAuthHeaderAttached(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, topologyJSON)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, BasicAuthTransport("demo", "demo"))
	if _, err := c.Topology(context.Background()); err != nil {
		t.Fatal(err)
	}
	user, pass, ok := (&http.Request{Header: http.Header{"Authorization": {gotAuth}}}).BasicAuth()
	if !ok || user != "demo" || pass != "demo" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}

// oauthTestServer serves both the token endpoint (/token) and the API.
type oauthTestServer struct {
	t            *testing.T
	tokenFetches atomic.Int64
	apiHits      atomic.Int64
	expiresIn    *int64 // nil = omit the field

	mu        sync.Mutex
	apiBodies []string
	perToken  map[string]int
	failFirst string // token whose SECOND API use returns 401 (see handler)
}

func newOAuthServer(t *testing.T) (*oauthTestServer, *httptest.Server) {
	o := &oauthTestServer{t: t, perToken: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			if got := r.Header.Get("Authorization"); got != "" {
				t.Errorf("token endpoint received Authorization header %q (recursion!)", got)
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
				t.Errorf("token request Content-Type = %q", ct)
			}
			if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != "client_credentials" ||
				r.PostForm.Get("client_id") != "id" || r.PostForm.Get("client_secret") != "sec" ||
				r.PostForm.Get("audience") != "aud" {
				t.Errorf("bad token form: %v (%v)", r.PostForm, err)
			}
			n := o.tokenFetches.Add(1)
			resp := map[string]any{"access_token": fmt.Sprintf("tok%d", n)}
			if o.expiresIn != nil {
				resp["expires_in"] = *o.expiresIn
			} else {
				resp["expires_in"] = int64(3600)
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		o.apiHits.Add(1)
		body, _ := io.ReadAll(r.Body)
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		o.mu.Lock()
		o.apiBodies = append(o.apiBodies, string(body))
		o.perToken[token]++
		uses := o.perToken[token]
		fail := o.failFirst
		o.mu.Unlock()
		if token == fail && uses == 2 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/topology") {
			fmt.Fprint(w, topologyJSON)
		} else {
			fmt.Fprint(w, emptySearchJSON)
		}
	}))
	return o, srv
}

func oauthClient(srvURL string) *Client {
	return NewClient(srvURL, OAuthTransport(srvURL+"/token", "id", "sec", "aud", ""))
}

func TestOAuthTokenCachedAcrossConcurrentRequests(t *testing.T) {
	o, srv := newOAuthServer(t)
	defer srv.Close()
	c := oauthClient(srv.URL)

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var err error
			if i%2 == 0 {
				_, err = c.Topology(context.Background())
			} else {
				_, _, err = c.SearchProcessInstances(context.Background(), 10)
			}
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if got := o.tokenFetches.Load(); got != 1 {
		t.Fatalf("token fetches = %d, want 1", got)
	}
}

func TestOAuth401RetryReplaysIdenticalBody(t *testing.T) {
	o, srv := newOAuthServer(t)
	o.failFirst = "tok1" // tok1's second API use 401s; retry must use tok2
	defer srv.Close()
	c := oauthClient(srv.URL)

	// Call 1: fetches tok1 fresh, first use succeeds (fresh tokens are
	// deliberately not retried, so the 401 must hit a CACHED token).
	if _, _, err := c.SearchProcessInstances(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	// Call 2: cached tok1 → 401 → invalidate, fetch tok2, replay.
	if _, _, err := c.SearchProcessInstances(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if got := o.tokenFetches.Load(); got != 2 {
		t.Fatalf("token fetches = %d, want 2", got)
	}
	if got := o.apiHits.Load(); got != 3 {
		t.Fatalf("api hits = %d, want 3 (ok, 401, replay)", got)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	failed, replayed := o.apiBodies[1], o.apiBodies[2]
	if failed == "" || failed != replayed {
		t.Fatalf("replayed body differs:\n401'd:    %q\nreplayed: %q", failed, replayed)
	}
}

func TestOAuthExpiresInEdgesDoNotLoop(t *testing.T) {
	for name, exp := range map[string]*int64{
		"missing": nil,
		"zero":    ptr(int64(0)),
		"tiny":    ptr(int64(10)),
	} {
		t.Run(name, func(t *testing.T) {
			o, srv := newOAuthServer(t)
			o.expiresIn = exp
			defer srv.Close()
			c := oauthClient(srv.URL)
			for range 5 {
				if _, err := c.Topology(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if got := o.tokenFetches.Load(); got > 2 {
				t.Fatalf("token fetches = %d across 5 requests, want <= 2", got)
			}
		})
	}
}

func TestUnauthorizedErrorIsReadable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"huge":"json blob that should not reach the footer"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	_, err := c.Topology(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unauthorized (check credentials)") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "json blob") {
		t.Fatalf("error leaks response body: %v", err)
	}
}

func TestOAuthTokenEndpointErrorIsShort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_client","error_description":"Invalid client credentials"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, OAuthTransport(srv.URL+"/token", "id", "bad", "aud", ""))
	_, err := c.Topology(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid_client") || !strings.Contains(err.Error(), "Invalid client credentials") {
		t.Fatalf("err = %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
