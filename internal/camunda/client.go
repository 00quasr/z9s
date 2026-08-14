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
	return search[ProcessDefinition](ctx, c, "/v2/process-definitions/search", limit)
}

func (c *Client) SearchProcessInstances(ctx context.Context, limit int) ([]ProcessInstance, int, error) {
	return search[ProcessInstance](ctx, c, "/v2/process-instances/search", limit)
}

func (c *Client) SearchIncidents(ctx context.Context, limit int) ([]Incident, int, error) {
	return search[Incident](ctx, c, "/v2/incidents/search", limit)
}

func search[T any](ctx context.Context, c *Client, path string, limit int) ([]T, int, error) {
	body := map[string]any{"page": map[string]any{"limit": limit}}
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
