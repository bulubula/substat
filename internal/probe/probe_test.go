package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAssertions(t *testing.T) {
	ctx := &AssertionContext{
		Status:         200,
		Headers:        map[string]string{"content-type": "application/json; charset=utf-8"},
		Body:           `{"status":"ok","code":0}`,
		LatencyMs:      120,
		CertExpiryDays: 30,
	}

	tests := []struct {
		a    Assertion
		want bool
	}{
		{Assertion{Target: "status", Operator: "equals", Value: "200"}, true},
		{Assertion{Target: "status", Operator: "in", Value: "[200, 204]"}, true},
		{Assertion{Target: "status", Operator: "in", Value: "[400, 500]"}, false},
		{Assertion{Target: "header.content-type", Operator: "contains", Value: "json"}, true},
		{Assertion{Target: "body", Operator: "contains", Value: "ok"}, true},
		{Assertion{Target: "body", Operator: "matches_regex", Value: `"code":\s*0`}, true},
		{Assertion{Target: "latency_ms", Operator: "lte", Value: "200"}, true},
		{Assertion{Target: "latency_ms", Operator: "lte", Value: "50"}, false},
		{Assertion{Target: "cert_expiry_days", Operator: "gte", Value: "7"}, true},
		{Assertion{Target: "cert_expiry_days", Operator: "gte", Value: "60"}, false},
	}

	for _, tt := range tests {
		ok, _ := evaluateOne(tt.a, ctx)
		if ok != tt.want {
			t.Errorf("evaluateOne(%+v) = %v; want %v", tt.a, ok, tt.want)
		}
	}
}

func TestHTTPProbe(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"alive":true}`))
	}))
	defer ts.Close()

	p, err := Create("http", map[string]any{
		"target": ts.URL,
		"assert": []any{
			map[string]any{"target": "status", "operator": "equals", "value": "200"},
			map[string]any{"target": "body", "operator": "contains", "value": "alive"},
		},
	})
	if err != nil {
		t.Fatalf("Create http probe err: %v", err)
	}

	res := p.Execute(context.Background())
	if !res.Success {
		t.Fatalf("Probe execution failed: %s", res.Message)
	}
}
