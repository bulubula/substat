package probe

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type ChainStep struct {
	Name       string
	Target     string
	Method     string
	Headers    map[string]string
	Body       string
	Extract    map[string]string // e.g. token -> "json:data.token", session -> "header:set-cookie"
	Assertions []Assertion
}

type HTTPChainProbe struct {
	Steps              []ChainStep
	InsecureSkipVerify bool
}

func init() {
	Register("http_chain", func(params map[string]any) (Probe, error) {
		rawSteps, ok := params["steps"].([]any)
		if !ok || len(rawSteps) == 0 {
			return nil, fmt.Errorf("http_chain probe requires non-empty 'steps'")
		}
		insecure, _ := params["insecure_skip_verify"].(bool)

		var steps []ChainStep
		for idx, s := range rawSteps {
			m, ok := s.(map[string]any)
			if !ok {
				continue
			}
			stepName, _ := m["name"].(string)
			if stepName == "" {
				stepName = fmt.Sprintf("step-%d", idx+1)
			}
			target, _ := m["target"].(string)
			method, _ := m["method"].(string)
			if method == "" {
				method = "GET"
			}
			body, _ := m["body"].(string)

			headers := make(map[string]string)
			if rawHeaders, ok := m["headers"].(map[string]any); ok {
				for k, v := range rawHeaders {
					headers[k] = fmt.Sprint(v)
				}
			}

			extracts := make(map[string]string)
			if rawExtract, ok := m["extract"].(map[string]any); ok {
				for k, v := range rawExtract {
					extracts[k] = fmt.Sprint(v)
				}
			}

			var assertions []Assertion
			if rawAsserts, ok := m["assert"].([]any); ok {
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

			steps = append(steps, ChainStep{
				Name:       stepName,
				Target:     target,
				Method:     strings.ToUpper(method),
				Headers:    headers,
				Body:       body,
				Extract:    extracts,
				Assertions: assertions,
			})
		}

		return &HTTPChainProbe{
			Steps:              steps,
			InsecureSkipVerify: insecure,
		}, nil
	})
}

func (p *HTTPChainProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	res := Result{Timestamp: start, Details: make(map[string]any)}

	variables := make(map[string]string)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: p.InsecureSkipVerify},
		},
	}

	for _, step := range p.Steps {
		stepTarget := replaceVars(step.Target, variables)
		stepBody := replaceVars(step.Body, variables)

		var bodyReader io.Reader
		if stepBody != "" {
			bodyReader = bytes.NewBufferString(stepBody)
		}

		req, err := http.NewRequestWithContext(ctx, step.Method, stepTarget, bodyReader)
		if err != nil {
			res.Success = false
			res.Latency = time.Since(start)
			res.Message = fmt.Sprintf("[%s] build req failed: %v", step.Name, err)
			return res
		}

		for k, v := range step.Headers {
			req.Header.Set(k, replaceVars(v, variables))
		}

		stepStart := time.Now()
		resp, err := client.Do(req)
		stepLatency := time.Since(stepStart)

		if err != nil {
			res.Success = false
			res.Latency = time.Since(start)
			res.Message = fmt.Sprintf("[%s] request error: %v", step.Name, err)
			return res
		}

		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()
		respStr := string(respBytes)

		respHeaders := make(map[string]string)
		for k, v := range resp.Header {
			if len(v) > 0 {
				respHeaders[strings.ToLower(k)] = v[0]
			}
		}

		// Assertions
		assertCtx := &AssertionContext{
			Status:    resp.StatusCode,
			Headers:   respHeaders,
			Body:      respStr,
			LatencyMs: stepLatency.Milliseconds(),
		}
		ok, reason := EvaluateAssertions(step.Assertions, assertCtx)
		if !ok {
			res.Success = false
			res.Latency = time.Since(start)
			res.Message = fmt.Sprintf("[%s] assert fail: %s", step.Name, reason)
			return res
		}

		// Extractions
		for varName, expr := range step.Extract {
			extractedVal := extractValue(expr, respHeaders, respStr)
			if extractedVal != "" {
				variables[varName] = extractedVal
			}
		}
	}

	res.Success = true
	res.Latency = time.Since(start)
	res.Details["steps_completed"] = len(p.Steps)
	return res
}

func replaceVars(template string, vars map[string]string) string {
	out := template
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

func extractValue(expr string, headers map[string]string, body string) string {
	parts := strings.SplitN(expr, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	extractorType := strings.ToLower(parts[0])
	param := parts[1]

	switch extractorType {
	case "header":
		return headers[strings.ToLower(param)]
	case "json":
		var root any
		if err := json.Unmarshal([]byte(body), &root); err != nil {
			return ""
		}
		// simple single-level or multi-level dot path
		keys := strings.Split(param, ".")
		curr := root
		for _, k := range keys {
			if m, ok := curr.(map[string]any); ok {
				curr = m[k]
			} else {
				return ""
			}
		}
		return fmt.Sprint(curr)
	case "regex":
		re, err := regexp.Compile(param)
		if err != nil {
			return ""
		}
		match := re.FindStringSubmatch(body)
		if len(match) > 1 {
			return match[1]
		} else if len(match) == 1 {
			return match[0]
		}
	}
	return ""
}
