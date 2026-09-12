package probe

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

type DNSProbe struct {
	Server     string
	Domain     string
	QueryType  string
	Assertions []Assertion
}

func init() {
	Register("dns", func(params map[string]any) (Probe, error) {
		server, _ := params["server"].(string)
		if server == "" {
			server = "1.1.1.1:53"
		}
		if !strings.Contains(server, ":") {
			server = server + ":53"
		}

		domain, _ := params["domain"].(string)
		if domain == "" {
			return nil, fmt.Errorf("dns probe requires 'domain'")
		}

		qType, _ := params["query_type"].(string)
		if qType == "" {
			qType = "A"
		}

		var assertions []Assertion
		if rawAsserts, ok := params["assert"].([]any); ok {
			for _, item := range rawAsserts {
				if am, ok := item.(map[string]any); ok {
					assertions = append(assertions, Assertion{
						Target:   fmt.Sprint(am["target"]),
						Operator: fmt.Sprint(am["operator"]),
						Value:    fmt.Sprint(am["value"]),
					})
				}
			}
		}
		if len(assertions) == 0 {
			assertions = append(assertions, Assertion{
				Target:   "resolved",
				Operator: "equals",
				Value:    "true",
			})
		}

		return &DNSProbe{
			Server:     server,
			Domain:     domain,
			QueryType:  strings.ToUpper(qType),
			Assertions: assertions,
		}, nil
	})
}

func (p *DNSProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	res := Result{Timestamp: start, Details: make(map[string]any)}

	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", p.Server)
		},
	}

	ips, err := r.LookupHost(ctx, p.Domain)
	latency := time.Since(start)
	res.Latency = latency

	if err != nil {
		res.Success = false
		res.Message = fmt.Sprintf("dns lookup failed: %v", err)
		return res
	}

	res.Details["ips"] = ips
	ipListStr := strings.Join(ips, ",")

	assertCtx := &AssertionContext{
		Resolved:  len(ips) > 0,
		LatencyMs: latency.Milliseconds(),
		Body:      ipListStr,
	}

	ok, failureReason := EvaluateAssertions(p.Assertions, assertCtx)
	res.Success = ok
	if !ok {
		res.Message = failureReason
	}
	return res
}
