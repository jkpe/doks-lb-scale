package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PromClient queries a Prometheus server for an instant vector value using PromQL.
// When used via MuxMetrics, the metric string must be prefixed with "promql:".
type PromClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

type promAPIResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value []interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (p *PromClient) GetValue(ctx context.Context, _ string, metric string) (float64, error) {
	if p == nil || p.BaseURL == "" {
		return 0, errors.New("prometheus not configured")
	}
	if !strings.HasPrefix(metric, "promql:") {
		return 0, errors.New("metric must start with promql:")
	}
	query := strings.TrimSpace(strings.TrimPrefix(metric, "promql:"))
	if query == "" {
		return 0, errors.New("empty promql query")
	}

	// Build instant query: /api/v1/query?query=...&time=now
	endpoint := strings.TrimRight(p.BaseURL, "/") + "/api/v1/query"
	q := url.Values{}
	q.Set("query", query)
	q.Set("time", fmt.Sprintf("%d", time.Now().Unix()))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return 0, err
	}
	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("prometheus http %d", resp.StatusCode)
	}

	var parsed promAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, err
	}
	if strings.ToLower(parsed.Status) != "success" {
		return 0, errors.New("prometheus returned non-success status")
	}
	if len(parsed.Data.Result) == 0 {
		return 0, errors.New("prometheus: no result")
	}
	val := parsed.Data.Result[0].Value
	if len(val) < 2 {
		return 0, errors.New("prometheus: invalid value format")
	}
	var out float64
	switch v := val[1].(type) {
	case float64:
		out = v
	case string:
		if _, err := fmt.Sscan(v, &out); err != nil {
			return 0, err
		}
	default:
		return 0, errors.New("prometheus: unexpected value type")
	}
	return out, nil
}

// MuxMetrics routes to Prometheus when the metric begins with "promql:",
// otherwise routes to the DigitalOcean metrics client.
type MuxMetrics struct {
	DO   *DOClient
	Prom *PromClient
}

func (m *MuxMetrics) GetValue(ctx context.Context, lbID string, metric string) (float64, error) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(metric)), "promql:") {
		if m.Prom == nil {
			return 0, errors.New("prometheus client not configured")
		}
		return m.Prom.GetValue(ctx, lbID, metric)
	}
	if m.DO == nil {
		return 0, errors.New("digitalocean client not configured")
	}
	return m.DO.GetValue(ctx, lbID, metric)
}
