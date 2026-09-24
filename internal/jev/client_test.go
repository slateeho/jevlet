package jev

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecide(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth %q", got)
		}
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "jev-test" {
			t.Fatalf("unexpected model %q", req.Model)
		}
		if req.Questions["remediation"].Type != "choice" {
			t.Fatal("remediation must be a choice question")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"jev-1.13.0",
			"answers":{
				"probableCause":{"type":"choice","choice":"transient","confidence":0.94,"probabilities":{"transient":0.94,"unknown":0.06}},
				"remediation":{"type":"choice","choice":"restart_pod","confidence":0.97,"probabilities":{"restart_pod":0.97,"noop":0.02,"escalate":0.01}},
				"operationalRisk":{"type":"score","score":0.7,"confidence":0.91,"legend":{"0":"low","4":"critical"},"probabilities":{"0":0.5,"1":0.4,"2":0.1}},
				"safeToExecuteAutomatically":{"type":"noul","noul":0.98}
			},
			"usage":{"input_tokens":123,"output_tokens":9}
		}`))
	}))
	defer server.Close()

	c := &Client{BaseURL: server.URL, APIKey: "test-key", Model: "jev-test", HTTPClient: server.Client()}
	d, err := c.Decide(context.Background(), map[string]any{"pod": "x"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if d.Action != "restart_pod" || d.ActionConfidence != 0.97 {
		t.Fatalf("unexpected action: %+v", d)
	}
	if d.SafeToExecute != 0.98 || d.RiskScore != 0.7 {
		t.Fatalf("unexpected gate data: %+v", d)
	}
	if d.ResolvedModel != "jev-1.13.0" || d.Usage.InputTokens != 123 {
		t.Fatalf("unexpected metadata: %+v", d)
	}
}
