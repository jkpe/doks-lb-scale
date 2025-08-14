package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type doMetricsResponse struct {
	Data struct {
		Result []struct {
			Values [][]interface{} `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func (d *DOClient) GetValue(ctx context.Context, lbID string, metric string) (float64, error) {
	if d.APIToken == "" {
		return 0, errors.New("missing DO_API_TOKEN")
	}

	endpoint := fmt.Sprintf("https://api.digitalocean.com/v2/monitoring/metrics/load_balancer/%s", url.PathEscape(metric))
	q := url.Values{}
	end := time.Now().UTC()
	start := end.Add(-5 * time.Minute)
	q.Set("start", fmt.Sprintf("%d", start.Unix()))
	q.Set("end", fmt.Sprintf("%d", end.Unix()))
	q.Set("lb_id", lbID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+d.APIToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("do metrics http %d", resp.StatusCode)
	}

	var parsed doMetricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, err
	}
	if len(parsed.Data.Result) == 0 {
		return 0, errors.New("no metrics result")
	}
	values := parsed.Data.Result[0].Values
	if len(values) == 0 {
		return 0, errors.New("no datapoints")
	}
	last := values[len(values)-1]
	if len(last) < 2 {
		return 0, errors.New("invalid datapoint")
	}
	var val float64
	switch v := last[1].(type) {
	case float64:
		val = v
	case string:
		if _, err := fmt.Sscan(v, &val); err != nil {
			return 0, err
		}
	default:
		return 0, errors.New("unexpected value type")
	}
	return val, nil
}
