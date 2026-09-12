package probe

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Assertion struct {
	Target   string `yaml:"target" json:"target"`     // status, header.xxx, body, latency_ms, cert_expiry_days, connected, resolved, packet_loss
	Operator string `yaml:"operator" json:"operator"` // equals, not_equals, contains, not_contains, matches_regex, lte, gte, in
	Value    string `yaml:"value" json:"value"`
}

type AssertionContext struct {
	Status         int
	Headers        map[string]string
	Body           string
	LatencyMs      int64
	CertExpiryDays int64
	Connected      bool
	Resolved       bool
	PacketLoss     float64
}

func EvaluateAssertions(assertions []Assertion, ctx *AssertionContext) (bool, string) {
	for _, a := range assertions {
		ok, reason := evaluateOne(a, ctx)
		if !ok {
			return false, reason
		}
	}
	return true, ""
}

func evaluateOne(a Assertion, ctx *AssertionContext) (bool, string) {
	target := strings.ToLower(strings.TrimSpace(a.Target))
	op := strings.ToLower(strings.TrimSpace(a.Operator))
	expected := strings.TrimSpace(a.Value)

	var actualStr string
	var actualNum int64
	var isNum bool

	switch {
	case target == "status":
		actualNum = int64(ctx.Status)
		actualStr = strconv.FormatInt(actualNum, 10)
		isNum = true
	case target == "latency_ms":
		actualNum = ctx.LatencyMs
		actualStr = strconv.FormatInt(actualNum, 10)
		isNum = true
	case target == "cert_expiry_days":
		actualNum = ctx.CertExpiryDays
		actualStr = strconv.FormatInt(actualNum, 10)
		isNum = true
	case target == "connected":
		actualStr = strconv.FormatBool(ctx.Connected)
	case target == "resolved":
		actualStr = strconv.FormatBool(ctx.Resolved)
	case target == "packet_loss" || target == "packet_loss_percent":
		actualNum = int64(ctx.PacketLoss)
		actualStr = fmt.Sprintf("%.2f", ctx.PacketLoss)
		isNum = true
	case target == "body":
		actualStr = ctx.Body
	case strings.HasPrefix(target, "header."):
		hName := strings.TrimPrefix(target, "header.")
		if ctx.Headers != nil {
			actualStr = ctx.Headers[strings.ToLower(hName)]
		}
	default:
		return false, fmt.Sprintf("unsupported assert target %q", a.Target)
	}

	switch op {
	case "equals", "==":
		if actualStr != expected {
			return false, fmt.Sprintf("assert failed: target %s expected %s, got %s", a.Target, expected, actualStr)
		}
	case "not_equals", "!=":
		if actualStr == expected {
			return false, fmt.Sprintf("assert failed: target %s must not equal %s", a.Target, expected)
		}
	case "contains":
		if !strings.Contains(actualStr, expected) {
			return false, fmt.Sprintf("assert failed: target %s does not contain %s", a.Target, expected)
		}
	case "not_contains":
		if strings.Contains(actualStr, expected) {
			return false, fmt.Sprintf("assert failed: target %s contains forbidden %s", a.Target, expected)
		}
	case "matches_regex":
		matched, err := regexp.MatchString(expected, actualStr)
		if err != nil || !matched {
			return false, fmt.Sprintf("assert failed: target %s regex %s no match", a.Target, expected)
		}
	case "lte", "<=":
		if !isNum {
			return false, fmt.Sprintf("assert failed: target %s not numeric", a.Target)
		}
		expNum, err := strconv.ParseInt(expected, 10, 64)
		if err != nil || actualNum > expNum {
			return false, fmt.Sprintf("assert failed: target %s (%d) > %s", a.Target, actualNum, expected)
		}
	case "gte", ">=":
		if !isNum {
			return false, fmt.Sprintf("assert failed: target %s not numeric", a.Target)
		}
		expNum, err := strconv.ParseInt(expected, 10, 64)
		if err != nil || actualNum < expNum {
			return false, fmt.Sprintf("assert failed: target %s (%d) < %s", a.Target, actualNum, expected)
		}
	case "in":
		// Format: "[200, 204]" or "200, 204"
		clean := strings.Trim(expected, "[]")
		parts := strings.Split(clean, ",")
		found := false
		for _, p := range parts {
			if strings.TrimSpace(p) == actualStr {
				found = true
				break
			}
		}
		if !found {
			return false, fmt.Sprintf("assert failed: target %s (%s) not in %s", a.Target, actualStr, expected)
		}
	default:
		return false, fmt.Sprintf("unknown operator %q", a.Operator)
	}

	return true, ""
}
