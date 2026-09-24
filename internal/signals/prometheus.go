package signals

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type PrometheusClient struct{ HTTPClient *http.Client }

type promResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value []any `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (p PrometheusClient) Query(ctx context.Context, baseURL, query string) (float64, error) {
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/v1/query")
	if err != nil {
		return 0, err
	}
	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus status %d", resp.StatusCode)
	}
	var out promResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	if out.Status != "success" || len(out.Data.Result) == 0 || len(out.Data.Result[0].Value) < 2 {
		return 0, fmt.Errorf("prometheus returned no scalar/vector sample")
	}
	s, ok := out.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected prometheus sample value")
	}
	return strconv.ParseFloat(s, 64)
}

func RenderQuery(template, namespace, pod string) string {
	return strings.NewReplacer("{{namespace}}", namespace, "{{pod}}", pod).Replace(template)
}
