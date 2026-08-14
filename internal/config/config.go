// Package config resolves which Camunda cluster z9s talks to and how it
// authenticates. It reads c8ctl's profile store READ-ONLY (interop tested
// against c8ctl v3.3.0) rather than inventing another config format: any
// profile configured with `c8ctl add profile` just works in z9s.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// AuthMode is inferred from which credentials a profile carries,
// mirroring c8ctl: clientId+clientSecret → oauth, username+password →
// basic, otherwise none.
type AuthMode string

const (
	AuthNone  AuthMode = "none"
	AuthBasic AuthMode = "basic"
	AuthOAuth AuthMode = "oauth"
)

// Profile is a fully resolved connection.
type Profile struct {
	Name    string // profile name; "" when auth-less --addr was used
	Source  string // flag | session | env | default-profile | builtin | addr
	BaseURL string // normalized: no trailing slash, no /v2 suffix
	Auth    AuthMode

	Username string
	Password string

	ClientID     string
	ClientSecret string
	OAuthURL     string
	Audience     string
	Scope        string
}

// Label is the short form shown in the TUI header, e.g. "local (basic)".
// Empty for unauthenticated ad-hoc connections.
func (p Profile) Label() string {
	if p.Name == "" {
		return ""
	}
	return fmt.Sprintf("%s (%s)", p.Name, p.Auth)
}

// rawProfile matches c8ctl's profiles.json entries.
type rawProfile struct {
	Name         string `json:"name"`
	BaseURL      string `json:"baseUrl"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Audience     string `json:"audience"`
	OAuthURL     string `json:"oAuthUrl"`
	Scope        string `json:"scope"`
	Username     string `json:"username"`
	Password     string `json:"password"`
}

type profilesFile struct {
	Profiles []rawProfile `json:"profiles"`
}

type sessionFile struct {
	ActiveProfile string `json:"activeProfile"`
}

// Resolve picks the connection profile using c8ctl's precedence:
// --profile flag → session activeProfile → CAMUNDA_* env (only when
// CAMUNDA_BASE_URL is set) → profile "local" → built-in demo/demo
// fallback. Exception for credential safety: --addr WITHOUT --profile is
// an unauthenticated connection to exactly that address; --addr WITH
// --profile overrides only the address, keeping the profile's auth.
func Resolve(profileFlag, addrFlag string) (Profile, []string, error) {
	home, _ := os.UserHomeDir()
	r := resolver{getenv: os.Getenv, goos: runtime.GOOS, home: home}
	return r.resolve(profileFlag, addrFlag)
}

// resolver carries the injectable environment so tests can run hermetic.
type resolver struct {
	getenv func(string) string
	goos   string
	home   string
}

func (r resolver) resolve(profileFlag, addrFlag string) (Profile, []string, error) {
	if addrFlag != "" && profileFlag == "" {
		return Profile{Source: "addr", BaseURL: normalizeBaseURL(addrFlag), Auth: AuthNone}, nil, nil
	}

	profiles, session, warnings := r.load()

	prof, err := r.pick(profileFlag, profiles, session, &warnings)
	if err != nil {
		return Profile{}, warnings, err
	}
	if addrFlag != "" {
		prof.BaseURL = addrFlag
	}
	prof.BaseURL = normalizeBaseURL(prof.BaseURL)

	if prof.Auth == AuthOAuth && prof.OAuthURL == "" {
		return Profile{}, warnings, fmt.Errorf("profile %q has client credentials but no oAuthUrl (token endpoint)", prof.Name)
	}
	return prof, warnings, nil
}

func (r resolver) pick(profileFlag string, profiles []rawProfile, session sessionFile, warnings *[]string) (Profile, error) {
	byName := func(name string) *rawProfile {
		for i := range profiles {
			if profiles[i].Name == name {
				return &profiles[i]
			}
		}
		return nil
	}

	if profileFlag != "" {
		if strings.HasPrefix(profileFlag, "modeler:") {
			return Profile{}, fmt.Errorf("profile %q: Camunda Modeler-managed profiles are not supported by z9s", profileFlag)
		}
		p := byName(profileFlag)
		if p == nil {
			names := make([]string, 0, len(profiles))
			for _, pr := range profiles {
				names = append(names, pr.Name)
			}
			sort.Strings(names)
			return Profile{}, fmt.Errorf("unknown profile %q (have: %s)", profileFlag, strings.Join(names, ", "))
		}
		return fromRaw(*p, "flag"), nil
	}

	if session.ActiveProfile != "" {
		if p := byName(session.ActiveProfile); p != nil {
			return fromRaw(*p, "session"), nil
		}
		// Stale or Modeler-managed session state: fall through like c8ctl.
		*warnings = append(*warnings, fmt.Sprintf("active profile %q not found in profiles.json; falling back", session.ActiveProfile))
	}

	if r.getenv("CAMUNDA_BASE_URL") != "" {
		return fromRaw(rawProfile{
			Name:         "env",
			BaseURL:      r.getenv("CAMUNDA_BASE_URL"),
			ClientID:     r.getenv("CAMUNDA_CLIENT_ID"),
			ClientSecret: r.getenv("CAMUNDA_CLIENT_SECRET"),
			OAuthURL:     r.getenv("CAMUNDA_OAUTH_URL"),
			Audience:     r.getenv("CAMUNDA_TOKEN_AUDIENCE"),
			Scope:        r.getenv("CAMUNDA_OAUTH_SCOPE"),
			Username:     r.getenv("CAMUNDA_USERNAME"),
			Password:     r.getenv("CAMUNDA_PASSWORD"),
		}, "env"), nil
	}

	if p := byName("local"); p != nil {
		return fromRaw(*p, "default-profile"), nil
	}

	// c8ctl's own hardcoded fallback: c8run's default basic credentials.
	// An auth-disabled cluster ignores the header.
	return fromRaw(rawProfile{
		Name:     "local",
		BaseURL:  "http://localhost:8080/v2",
		Username: "demo",
		Password: "demo",
	}, "builtin"), nil
}

func fromRaw(p rawProfile, source string) Profile {
	out := Profile{
		Name:         p.Name,
		Source:       source,
		BaseURL:      p.BaseURL,
		Username:     p.Username,
		Password:     p.Password,
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		OAuthURL:     p.OAuthURL,
		Audience:     p.Audience,
		Scope:        p.Scope,
	}
	switch {
	case p.ClientID != "" && p.ClientSecret != "":
		out.Auth = AuthOAuth
	case p.Username != "" && p.Password != "":
		out.Auth = AuthBasic
	default:
		out.Auth = AuthNone
	}
	return out
}

// load reads c8ctl's profiles.json and session.json, tolerating absence
// and corruption (fall through with a warning, like c8ctl does).
func (r resolver) load() ([]rawProfile, sessionFile, []string) {
	var warnings []string
	dir := dataDir(r.goos, r.getenv, r.home)

	var pf profilesFile
	profilesPath := filepath.Join(dir, "profiles.json")
	if b, err := os.ReadFile(profilesPath); err == nil {
		if err := json.Unmarshal(b, &pf); err != nil {
			warnings = append(warnings, fmt.Sprintf("ignoring unreadable %s: %v", profilesPath, err))
		}
	}

	var sf sessionFile
	sessionPath := filepath.Join(dir, "session.json")
	if b, err := os.ReadFile(sessionPath); err == nil {
		if err := json.Unmarshal(b, &sf); err != nil {
			warnings = append(warnings, fmt.Sprintf("ignoring unreadable %s: %v", sessionPath, err))
		}
	}
	return pf.Profiles, sf, warnings
}

// dataDir mirrors c8ctl's per-platform data directory scheme.
func dataDir(goos string, getenv func(string) string, home string) string {
	if v := getenv("C8CTL_DATA_DIR"); v != "" {
		return v
	}
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "c8ctl")
	case "windows":
		if v := getenv("APPDATA"); v != "" {
			return filepath.Join(v, "c8ctl")
		}
		return filepath.Join(home, "AppData", "Roaming", "c8ctl")
	default:
		if v := getenv("XDG_CONFIG_HOME"); v != "" {
			return filepath.Join(v, "c8ctl")
		}
		return filepath.Join(home, ".config", "c8ctl")
	}
}

// normalizeBaseURL strips trailing slashes and a trailing /v2 segment
// (case-insensitive, matching the official SDK's regex) — the client
// appends /v2/... itself.
func normalizeBaseURL(u string) string {
	u = strings.TrimRight(u, "/")
	if i := strings.LastIndex(u, "/"); i >= 0 && strings.EqualFold(u[i+1:], "v2") {
		u = u[:i]
	}
	return strings.TrimRight(u, "/")
}
