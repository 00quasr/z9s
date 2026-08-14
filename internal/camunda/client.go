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

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
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

func (c *Client) SearchIncidents(ctx context.Context, limit int) ([]Incident, int, error) {
	return search[Incident](ctx, c, "/v2/incidents/search", nil, limit)
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
	return search[Incident](ctx, c, "/v2/incidents/search", byInstance(processInstanceKey), 100)
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
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, msg)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
