// Package camunda is a minimal client for the Camunda 8 Orchestration
// Cluster REST API (v2). It covers only what z9s renders; it is not a
// general-purpose SDK.
package camunda

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a client for the cluster at baseURL (no trailing /v2).
// rt attaches authentication (see auth.go); nil means unauthenticated.
func NewClient(baseURL string, rt http.RoundTripper) *Client {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			// Generous enough for a cold start that includes an OAuth
			// token round-trip (bounded separately at 10s) plus the call.
			Timeout:   15 * time.Second,
			Transport: rt,
			// Auth headers are injected at the transport layer, so refuse
			// cross-host redirects rather than leak credentials.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 0 && req.URL.Host != via[0].URL.Host {
					return fmt.Errorf("refusing cross-host redirect to %s", req.URL.Host)
				}
				return nil
			},
		},
	}
}

type Topology struct {
	Brokers           []Broker `json:"brokers"`
	ClusterID         string   `json:"clusterId"`
	ClusterSize       int      `json:"clusterSize"`
	PartitionsCount   int      `json:"partitionsCount"`
	ReplicationFactor int      `json:"replicationFactor"`
	GatewayVersion    string   `json:"gatewayVersion"`
}

type Broker struct {
	NodeID     int         `json:"nodeId"`
	Host       string      `json:"host"`
	Port       int         `json:"port"`
	Partitions []Partition `json:"partitions"`
	Version    string      `json:"version"`
}

type Partition struct {
	PartitionID int    `json:"partitionId"`
	Role        string `json:"role"`
	Health      string `json:"health"`
}

type ProcessDefinition struct {
	Name                 string `json:"name"`
	ResourceName         string `json:"resourceName"`
	Version              int    `json:"version"`
	ProcessDefinitionID  string `json:"processDefinitionId"`
	TenantID             string `json:"tenantId"`
	ProcessDefinitionKey string `json:"processDefinitionKey"`
}

type ProcessInstance struct {
	ProcessDefinitionID      string  `json:"processDefinitionId"`
	ProcessDefinitionName    string  `json:"processDefinitionName"`
	ProcessDefinitionVersion int     `json:"processDefinitionVersion"`
	StartDate                string  `json:"startDate"`
	EndDate                  *string `json:"endDate"`
	State                    string  `json:"state"`
	HasIncident              bool    `json:"hasIncident"`
	TenantID                 string  `json:"tenantId"`
	ProcessInstanceKey       string  `json:"processInstanceKey"`
	ProcessDefinitionKey     string  `json:"processDefinitionKey"`
}

type ElementInstance struct {
	ProcessDefinitionID string  `json:"processDefinitionId"`
	StartDate           string  `json:"startDate"`
	EndDate             *string `json:"endDate"`
	ElementID           string  `json:"elementId"`
	ElementName         string  `json:"elementName"`
	Type                string  `json:"type"`
	State               string  `json:"state"`
	HasIncident         bool    `json:"hasIncident"`
	ElementInstanceKey  string  `json:"elementInstanceKey"`
	ProcessInstanceKey  string  `json:"processInstanceKey"`
	IncidentKey         *string `json:"incidentKey"`
}

type Variable struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	IsTruncated bool   `json:"isTruncated"`
	VariableKey string `json:"variableKey"`
	ScopeKey    string `json:"scopeKey"`
}

type Incident struct {
	ProcessDefinitionID string `json:"processDefinitionId"`
	ErrorType           string `json:"errorType"`
	ErrorMessage        string `json:"errorMessage"`
	ElementID           string `json:"elementId"`
	CreationTime        string `json:"creationTime"`
	State               string `json:"state"`
	IncidentKey         string `json:"incidentKey"`
	ProcessInstanceKey  string `json:"processInstanceKey"`
	JobKey              string `json:"jobKey"`
}

type page struct {
	TotalItems int `json:"totalItems"`
}

type searchResult[T any] struct {
	Items []T  `json:"items"`
	Page  page `json:"page"`
}

func (c *Client) Topology(ctx context.Context) (*Topology, error) {
	var t Topology
	if err := c.do(ctx, http.MethodGet, "/v2/topology", nil, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) SearchProcessDefinitions(ctx context.Context, limit int) ([]ProcessDefinition, int, error) {
	return search[ProcessDefinition](ctx, c, "/v2/process-definitions/search", nil, limit)
}

func (c *Client) SearchProcessInstances(ctx context.Context, limit int) ([]ProcessInstance, int, error) {
	return search[ProcessInstance](ctx, c, "/v2/process-instances/search", nil, limit)
}

// SearchIncidents returns unresolved incidents. Resolved ones linger in
// the secondary-storage index with state RESOLVED, so filter them out.
func (c *Client) SearchIncidents(ctx context.Context, limit int) ([]Incident, int, error) {
	return search[Incident](ctx, c, "/v2/incidents/search", map[string]any{"state": "ACTIVE"}, limit)
}

// GetProcessInstance fetches a single instance via a key-filtered search.
func (c *Client) GetProcessInstance(ctx context.Context, key string) (*ProcessInstance, error) {
	items, _, err := search[ProcessInstance](ctx, c, "/v2/process-instances/search", byInstance(key), 1)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("process instance %s not found", key)
	}
	return &items[0], nil
}

func (c *Client) SearchElementInstances(ctx context.Context, processInstanceKey string) ([]ElementInstance, int, error) {
	return search[ElementInstance](ctx, c, "/v2/element-instances/search", byInstance(processInstanceKey), 100)
}

func (c *Client) SearchVariables(ctx context.Context, processInstanceKey string) ([]Variable, int, error) {
	return search[Variable](ctx, c, "/v2/variables/search", byInstance(processInstanceKey), 100)
}

func (c *Client) SearchInstanceIncidents(ctx context.Context, processInstanceKey string) ([]Incident, int, error) {
	filter := map[string]any{"processInstanceKey": processInstanceKey, "state": "ACTIVE"}
	return search[Incident](ctx, c, "/v2/incidents/search", filter, 100)
}

// ResolveIncident marks an incident as resolved so the engine retries the
// failed element. For job-related incidents the job must have retries
// again first, or the incident immediately reappears.
func (c *Client) ResolveIncident(ctx context.Context, incidentKey, jobKey string) error {
	if jobKey != "" {
		body := map[string]any{"changeset": map[string]any{"retries": 3}}
		if err := c.do(ctx, http.MethodPatch, "/v2/jobs/"+jobKey, body, nil); err != nil {
			return err
		}
	}
	return c.do(ctx, http.MethodPost, "/v2/incidents/"+incidentKey+"/resolution", struct{}{}, nil)
}

func (c *Client) CancelProcessInstance(ctx context.Context, processInstanceKey string) error {
	return c.do(ctx, http.MethodPost, "/v2/process-instances/"+processInstanceKey+"/cancellation", struct{}{}, nil)
}

// CreateProcessInstance starts an instance of the exact definition version
// behind processDefinitionKey and returns the new instance key.
func (c *Client) CreateProcessInstance(ctx context.Context, processDefinitionKey string) (string, error) {
	return c.createInstance(ctx, map[string]any{"processDefinitionKey": processDefinitionKey})
}

// CreateProcessInstanceByID starts an instance of the latest version of the
// given process (BPMN process id), with optional start variables.
func (c *Client) CreateProcessInstanceByID(ctx context.Context, processDefinitionID string, variables map[string]any) (string, error) {
	body := map[string]any{"processDefinitionId": processDefinitionID}
	if variables != nil {
		body["variables"] = variables
	}
	return c.createInstance(ctx, body)
}

func (c *Client) createInstance(ctx context.Context, body map[string]any) (string, error) {
	var res struct {
		ProcessInstanceKey string `json:"processInstanceKey"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/process-instances", body, &res); err != nil {
		return "", err
	}
	return res.ProcessInstanceKey, nil
}

// Job is an activated job as returned by /v2/jobs/activation.
type Job struct {
	JobKey             string         `json:"jobKey"`
	Type               string         `json:"type"`
	ProcessInstanceKey string         `json:"processInstanceKey"`
	ElementID          string         `json:"elementId"`
	Retries            int            `json:"retries"`
	Variables          map[string]any `json:"variables"`
}

func (c *Client) ActivateJobs(ctx context.Context, jobType, worker string, maxJobs int, timeout time.Duration) ([]Job, error) {
	body := map[string]any{
		"type":              jobType,
		"worker":            worker,
		"maxJobsToActivate": maxJobs,
		"timeout":           timeout.Milliseconds(),
	}
	var res struct {
		Jobs []Job `json:"jobs"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/jobs/activation", body, &res); err != nil {
		return nil, err
	}
	return res.Jobs, nil
}

func (c *Client) CompleteJob(ctx context.Context, jobKey string, variables map[string]any) error {
	body := map[string]any{}
	if variables != nil {
		body["variables"] = variables
	}
	return c.do(ctx, http.MethodPost, "/v2/jobs/"+jobKey+"/completion", body, nil)
}

func (c *Client) FailJob(ctx context.Context, jobKey string, retries int, errorMessage string) error {
	body := map[string]any{"retries": retries, "errorMessage": errorMessage}
	return c.do(ctx, http.MethodPost, "/v2/jobs/"+jobKey+"/failure", body, nil)
}

// ThrowJobError raises a BPMN error that a boundary event can catch.
func (c *Client) ThrowJobError(ctx context.Context, jobKey, errorCode, errorMessage string) error {
	body := map[string]any{"errorCode": errorCode, "errorMessage": errorMessage}
	return c.do(ctx, http.MethodPost, "/v2/jobs/"+jobKey+"/error", body, nil)
}

func byInstance(key string) map[string]any {
	return map[string]any{"processInstanceKey": key}
}

func search[T any](ctx context.Context, c *Client, path string, filter map[string]any, limit int) ([]T, int, error) {
	body := map[string]any{"page": map[string]any{"limit": limit}}
	if filter != nil {
		body["filter"] = filter
	}
	var res searchResult[T]
	if err := c.do(ctx, http.MethodPost, path, body, &res); err != nil {
		return nil, 0, err
	}
	return res.Items, res.Page.TotalItems, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%s %s: unauthorized (check credentials): %s", method, path, resp.Status)
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, msg)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
