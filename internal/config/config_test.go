package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture builds a temp c8ctl data dir and returns a resolver wired to it.
// files maps filename → JSON content; env adds CAMUNDA_* style variables.
func fixture(t *testing.T, files map[string]string, env map[string]string) resolver {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return resolver{
		goos: "darwin",
		home: "/home/irrelevant",
		getenv: func(key string) string {
			if key == "C8CTL_DATA_DIR" {
				return dir
			}
			return env[key]
		},
	}
}

const profilesWithLocalAndWork = `{"profiles":[
	{"name":"local","baseUrl":"http://localhost:8080/v2","username":"demo","password":"demo"},
	{"name":"work","baseUrl":"https://camunda.corp.example/v2","clientId":"z9s","clientSecret":"s3cret","oAuthUrl":"https://idp.corp.example/token","audience":"camunda-api"}
]}`

func TestNoSessionFileFallsBackToLocalProfile(t *testing.T) {
	// The common real-world state: profiles.json exists, session.json doesn't.
	r := fixture(t, map[string]string{"profiles.json": profilesWithLocalAndWork}, nil)
	p, warnings, err := r.resolve("", "")
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err=%v warnings=%v", err, warnings)
	}
	if p.Name != "local" || p.Source != "default-profile" || p.Auth != AuthBasic || p.BaseURL != "http://localhost:8080" {
		t.Fatalf("got %+v", p)
	}
}

func TestNoFilesUsesBuiltinDemoFallback(t *testing.T) {
	r := fixture(t, nil, nil)
	p, _, err := r.resolve("", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != "builtin" || p.Auth != AuthBasic || p.Username != "demo" || p.Password != "demo" || p.BaseURL != "http://localhost:8080" {
		t.Fatalf("got %+v", p)
	}
}

func TestCorruptProfilesWarnsAndFallsThrough(t *testing.T) {
	r := fixture(t, map[string]string{"profiles.json": "{not json"}, nil)
	p, warnings, err := r.resolve("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "profiles.json") {
		t.Fatalf("warnings=%v", warnings)
	}
	if p.Source != "builtin" {
		t.Fatalf("got %+v", p)
	}
}

func TestSessionActiveProfileWins(t *testing.T) {
	r := fixture(t, map[string]string{
		"profiles.json": profilesWithLocalAndWork,
		"session.json":  `{"activeProfile":"work"}`,
	}, map[string]string{"CAMUNDA_BASE_URL": "http://env-cluster:8080"})
	p, _, err := r.resolve("", "")
	if err != nil {
		t.Fatal(err)
	}
	// c8ctl's documented precedence: session profile beats CAMUNDA_* env.
	if p.Name != "work" || p.Source != "session" || p.Auth != AuthOAuth {
		t.Fatalf("got %+v", p)
	}
}

func TestStaleActiveProfileWarnsAndFallsThrough(t *testing.T) {
	r := fixture(t, map[string]string{
		"profiles.json": profilesWithLocalAndWork,
		"session.json":  `{"activeProfile":"gone"}`,
	}, nil)
	p, warnings, err := r.resolve("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"gone"`) {
		t.Fatalf("warnings=%v", warnings)
	}
	if p.Name != "local" || p.Source != "default-profile" {
		t.Fatalf("got %+v", p)
	}
}

func TestEnvGateRequiresBaseURL(t *testing.T) {
	// CAMUNDA_USERNAME alone must NOT engage the env path (c8ctl parity).
	r := fixture(t, map[string]string{"profiles.json": profilesWithLocalAndWork},
		map[string]string{"CAMUNDA_USERNAME": "alice", "CAMUNDA_PASSWORD": "pw"})
	p, _, err := r.resolve("", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != "default-profile" || p.Username != "demo" {
		t.Fatalf("got %+v", p)
	}
}

func TestEnvProfileEngagesOnBaseURL(t *testing.T) {
	r := fixture(t, nil, map[string]string{
		"CAMUNDA_BASE_URL": "http://env-cluster:8080/v2/",
		"CAMUNDA_USERNAME": "alice",
		"CAMUNDA_PASSWORD": "pw",
	})
	p, _, err := r.resolve("", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != "env" || p.Auth != AuthBasic || p.BaseURL != "http://env-cluster:8080" || p.Username != "alice" {
		t.Fatalf("got %+v", p)
	}
}

func TestEmptyClientIDMeansNoOAuth(t *testing.T) {
	r := fixture(t, map[string]string{"profiles.json": `{"profiles":[
		{"name":"local","baseUrl":"http://localhost:8080","clientId":"","clientSecret":"x"}
	]}`}, nil)
	p, _, err := r.resolve("", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Auth != AuthNone {
		t.Fatalf("got %+v", p)
	}
}

func TestUnknownProfileFlagErrorsListingNames(t *testing.T) {
	r := fixture(t, map[string]string{"profiles.json": profilesWithLocalAndWork}, nil)
	_, _, err := r.resolve("prod", "")
	if err == nil || !strings.Contains(err.Error(), `unknown profile "prod"`) || !strings.Contains(err.Error(), "local, work") {
		t.Fatalf("err=%v", err)
	}
}

func TestModelerProfileRejected(t *testing.T) {
	r := fixture(t, nil, nil)
	_, _, err := r.resolve("modeler:foo", "")
	if err == nil || !strings.Contains(err.Error(), "Modeler") {
		t.Fatalf("err=%v", err)
	}
}

func TestAddrAloneIsUnauthenticated(t *testing.T) {
	// Credential-safety rule: --addr without --profile never carries auth.
	r := fixture(t, map[string]string{"profiles.json": profilesWithLocalAndWork}, nil)
	p, _, err := r.resolve("", "https://prod.example.com/v2")
	if err != nil {
		t.Fatal(err)
	}
	if p.Auth != AuthNone || p.Source != "addr" || p.BaseURL != "https://prod.example.com" || p.Name != "" {
		t.Fatalf("got %+v", p)
	}
}

func TestAddrWithProfileOverridesAddressKeepsAuth(t *testing.T) {
	r := fixture(t, map[string]string{"profiles.json": profilesWithLocalAndWork}, nil)
	p, _, err := r.resolve("work", "https://staging.corp.example")
	if err != nil {
		t.Fatal(err)
	}
	if p.Auth != AuthOAuth || p.BaseURL != "https://staging.corp.example" || p.ClientID != "z9s" {
		t.Fatalf("got %+v", p)
	}
}

func TestOAuthWithoutTokenURLErrors(t *testing.T) {
	r := fixture(t, map[string]string{"profiles.json": `{"profiles":[
		{"name":"broken","baseUrl":"http://x","clientId":"a","clientSecret":"b"}
	]}`}, nil)
	_, _, err := r.resolve("broken", "")
	if err == nil || !strings.Contains(err.Error(), "oAuthUrl") {
		t.Fatalf("err=%v", err)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"http://localhost:8080/v2":               "http://localhost:8080",
		"http://localhost:8080/v2/":              "http://localhost:8080",
		"http://localhost:8080/V2":               "http://localhost:8080",
		"http://localhost:8080":                  "http://localhost:8080",
		"https://xxx-1.api.camunda.io/abc-123/":  "https://xxx-1.api.camunda.io/abc-123",
		"https://proxy.corp/camunda/v2":          "https://proxy.corp/camunda",
	}
	for in, want := range cases {
		if got := normalizeBaseURL(in); got != want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDataDirPerPlatform(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	cases := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{"override", "linux", map[string]string{"C8CTL_DATA_DIR": "/custom"}, "/custom"},
		{"darwin", "darwin", nil, "/home/u/Library/Application Support/c8ctl"},
		{"windows-appdata", "windows", map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`}, filepath.Join(`C:\Users\u\AppData\Roaming`, "c8ctl")},
		{"linux-xdg", "linux", map[string]string{"XDG_CONFIG_HOME": "/xdg"}, "/xdg/c8ctl"},
		{"linux-default", "linux", nil, "/home/u/.config/c8ctl"},
	}
	for _, c := range cases {
		if got := dataDir(c.goos, env(c.env), "/home/u"); got != c.want {
			t.Errorf("%s: dataDir = %q, want %q", c.name, got, c.want)
		}
	}
}
