package probe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"time"
)

type PingProbe struct {
	Target     string
	Count      int
	Assertions []Assertion
}

func init() {
	Register("ping", func(params map[string]any) (Probe, error) {
		target, _ := params["target"].(string)
		if target == "" {
			return nil, fmt.Errorf("ping probe requires 'target'")
		}
		count := 3
		if c, ok := params["count"].(int); ok && c > 0 {
			count = c
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
				Target:   "packet_loss",
				Operator: "equals",
				Value:    "0",
			})
		}

		return &PingProbe{
			Target:     target,
			Count:      count,
			Assertions: assertions,
		}, nil
	})
}

func (p *PingProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	res := Result{Timestamp: start, Details: make(map[string]any)}

	var cmd *exec.Cmd
	pingBin, err := exec.LookPath("ping")
	if err != nil {
		if _, statErr := os.Stat("/usr/bin/ping"); statErr == nil {
			pingBin = "/usr/bin/ping"
		} else if _, statErr := os.Stat("/bin/ping"); statErr == nil {
			pingBin = "/bin/ping"
		} else {
			pingBin = "ping"
		}
	}

	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, pingBin, "-n", strconv.Itoa(p.Count), p.Target)
	} else {
		cmd = exec.CommandContext(ctx, pingBin, "-c", strconv.Itoa(p.Count), "-W", "2", p.Target)
	}

	outBytes, err := cmd.CombinedOutput()
	latency := time.Since(start)
	res.Latency = latency

	out := string(outBytes)
	packetLoss := parsePingLoss(out)
	avgRtt := parsePingAvgRtt(out)

	if avgRtt > 0 {
		res.Latency = time.Duration(avgRtt * float64(time.Millisecond))
	}

	res.Details["packet_loss_percent"] = packetLoss
	res.Details["avg_rtt_ms"] = avgRtt

	if err != nil && packetLoss >= 100.0 {
		res.Success = false
		res.Message = fmt.Sprintf("ping unreachable: %v", err)
		return res
	}

	assertCtx := &AssertionContext{
		PacketLoss: packetLoss,
		LatencyMs:  int64(avgRtt),
		Body:       out,
	}

	ok, failureReason := EvaluateAssertions(p.Assertions, assertCtx)
	res.Success = ok
	if !ok {
		res.Message = failureReason
	}
	return res
}

// parses e.g. "0% packet loss" or "0.0% packet loss"
var lossRegex = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\%\s*(?:packet\s*)?loss`)

func parsePingLoss(out string) float64 {
	m := lossRegex.FindStringSubmatch(out)
	if len(m) > 1 {
		val, err := strconv.ParseFloat(m[1], 64)
		if err == nil {
			return val
		}
	}
	return 100.0
}

// parses rtt min/avg/max/mdev = 0.044/0.044/0.044/0.000 ms
var avgRegex = regexp.MustCompile(`(?:rtt|round-trip)\s*min/avg/max/(?:mdev|stddev)\s*=\s*[0-9\.]+/([0-9\.]+)/`)

func parsePingAvgRtt(out string) float64 {
	m := avgRegex.FindStringSubmatch(out)
	if len(m) > 1 {
		val, err := strconv.ParseFloat(m[1], 64)
		if err == nil {
			return val
		}
	}
	return 0
}
