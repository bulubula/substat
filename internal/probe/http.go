package probe

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPProbe struct {
	Target             string
	Method             string
	Headers            map[string]string
	Body               string
	InsecureSkipVerify bool
	Assertions         []Assertion
}

func init() {
	Register("http", func(params map[string]any) (Probe, error) {
		target, _ := params["target"].(string)
		if target == "" {
			return nil, fmt.Errorf("http probe requires 'target'")
		}
		method, _ := params["method"].(string)
		if method == "" {
			method = "GET"
		}
		insecure, _ := params["insecure_skip_verify"].(bool)
		body, _ := params["body"].(string)

		headers := make(map[string]string)
		if rawHeaders, ok := params["headers"].(map[string]any); ok {
			for k, v := range rawHeaders {
				headers[k] = fmt.Sprint(v)
			}
		}

		var assertions []Assertion
		if rawAsserts, ok := params["assert"].([]any); ok {
			for _, item := range rawAsserts {
				if m, ok := item.(map[string]any); ok {
					a := Assertion{
						Target:   fmt.Sprint(m["target"]),
						Operator: fmt.Sprint(m["operator"]),
						Value:    fmt.Sprint(m["value"]),
					}
					assertions = append(assertions, a)
				}
			}
		}
		// Default assert if empty
		if len(assertions) == 0 {
			assertions = append(assertions, Assertion{
				Target:   "status",
				Operator: "equals",
				Value:    "200",
			})
		}

		return &HTTPProbe{
			Target:             target,
			Method:             strings.ToUpper(method),
			Headers:            headers,
			Body:               body,
			InsecureSkipVerify: insecure,
			Assertions:         assertions,
		}, nil
	})
}

func (p *HTTPProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	res := Result{Timestamp: start, Details: make(map[string]any)}

	var bodyReader io.Reader
	if p.Body != "" {
		bodyReader = bytes.NewBufferString(p.Body)
	}

	req, err := http.NewRequestWithContext(ctx, p.Method, p.Target, bodyReader)
	if err != nil {
		res.Success = false
		res.Latency = time.Since(start)
		res.Message = fmt.Sprintf("failed to build request: %v", err)
		return res
	}

	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: p.InsecureSkipVerify},
	}
	client := &http.Client{
		Transport: tr,
	}

	resp, err := client.Do(req)
	latency := time.Since(start)
	res.Latency = latency

	if err != nil {
		res.Success = false
		res.Message = err.Error()
		return res
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	respBody := string(respBytes)

	respHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders[strings.ToLower(k)] = v[0]
		}
	}

	var certExpiryDays int64 = -1
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		certExpiryDays = int64(time.Until(cert.NotAfter).Hours() / 24)
		res.Details["cert_expiry_days"] = certExpiryDays
		res.Details["cert_subject"] = cert.Subject.CommonName
	}
	res.Details["status_code"] = resp.StatusCode

	assertCtx := &AssertionContext{
		Status:         resp.StatusCode,
		Headers:        respHeaders,
		Body:           respBody,
		LatencyMs:      latency.Milliseconds(),
		CertExpiryDays: certExpiryDays,
	}

	ok, failureReason := EvaluateAssertions(p.Assertions, assertCtx)
	res.Success = ok
	if !ok {
		res.Message = failureReason
	}
	return res
}
