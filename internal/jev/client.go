package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

type Question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
}

type Request struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Answer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Score         float64            `json:"score,omitempty"`
	Legend        map[string]any     `json:"legend,omitempty"`
	Noul          float64            `json:"noul,omitempty"`
}

type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

type Decision struct {
	RequestedModel   string
	ResolvedModel    string
	ProbableCause    string
	CauseConfidence  float64
	Action           string
	ActionConfidence float64
	RiskScore        float64
	RiskConfidence   float64
	SafeToExecute    float64
	Usage            Usage
	Latency          time.Duration
}

func (c *Client) Decide(ctx context.Context, state any, modelOverride string) (Decision, error) {
	model := strings.TrimSpace(modelOverride)
	if model == "" {
		model = strings.TrimSpace(c.Model)
	}
	if model == "" {
		model = "jev-latest"
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return Decision{}, errors.New("TYPESAFE_API_KEY is empty")
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://api.typesafe.ai"
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	reqBody := Request{
		State: state,
		Model: model,
		Questions: map[string]Question{
			"probableCause": {
				Type:         "choice",
				Instructions: "What is the most likely operational cause of this unhealthy Kubernetes Pod state?",
				Criteria: map[string]string{
					"application_regression": "The workload or newly deployed application behavior is the likely cause.",
					"resource_pressure":      "CPU, memory, or node resource pressure is the likely cause.",
					"dependency_failure":     "A downstream or infrastructure dependency is the likely cause.",
					"configuration":          "Configuration, Secret, volume, image, or startup configuration is the likely cause.",
					"transient":              "The evidence suggests a temporary condition likely to clear without changing configuration.",
					"unknown":                "The supplied state is insufficient to identify a cause reliably.",
				},
			},
			"remediation": {
				Type:         "choice",
				Instructions: "Choose the safest bounded action for the current Pod. Prefer noop or escalate when evidence is insufficient.",
				Criteria: map[string]string{
					"noop":        "Take no Kubernetes mutation; continue observing.",
					"escalate":    "Require a human or external incident workflow.",
					"restart_pod": "Delete only this Pod so its existing controller recreates it; use only for likely transient or stuck runtime state.",
				},
			},
			"operationalRisk": {
				Type:         "score",
				Instructions: "How risky is it to automatically execute the selected remediation right now?",
				Criteria: []string{
					"0: negligible risk; action is routine and easily reversible",
					"1: low risk",
					"2: moderate risk; automation should be conservative",
					"3: high risk; human review is preferable",
					"4: critical risk; do not automate",
				},
			},
			"safeToExecuteAutomatically": {
				Type:         "noul",
				Instructions: "Given only the supplied state and the selected bounded remediation, is it safe to execute automatically?",
				Criteria: map[string]string{
					"true":  "Evidence is sufficient and the selected action is bounded, reversible, and unlikely to increase impact.",
					"false": "Evidence is incomplete, the action could increase impact, or human review should happen first.",
				},
			},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return Decision{}, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/systemone", bytes.NewReader(payload))
	if err != nil {
		return Decision{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(started)
	if err != nil {
		return Decision{}, fmt.Errorf("typesafe request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return Decision{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Decision{}, fmt.Errorf("typesafe status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out Response
	if err := json.Unmarshal(body, &out); err != nil {
		return Decision{}, fmt.Errorf("decode response: %w", err)
	}

	cause, ok := out.Answers["probableCause"]
	if !ok || cause.Type != "choice" {
		return Decision{}, errors.New("missing probableCause choice answer")
	}
	action, ok := out.Answers["remediation"]
	if !ok || action.Type != "choice" {
		return Decision{}, errors.New("missing remediation choice answer")
	}
	risk, ok := out.Answers["operationalRisk"]
	if !ok || risk.Type != "score" {
		return Decision{}, errors.New("missing operationalRisk score answer")
	}
	safe, ok := out.Answers["safeToExecuteAutomatically"]
	if !ok || safe.Type != "noul" {
		return Decision{}, errors.New("missing safeToExecuteAutomatically noul answer")
	}

	return Decision{
		RequestedModel:   model,
		ResolvedModel:    out.Model,
		ProbableCause:    cause.Choice,
		CauseConfidence:  cause.Confidence,
		Action:           action.Choice,
		ActionConfidence: action.Confidence,
		RiskScore:        risk.Score,
		RiskConfidence:   risk.Confidence,
		SafeToExecute:    safe.Noul,
		Usage:            out.Usage,
		Latency:          latency,
	}, nil
}
