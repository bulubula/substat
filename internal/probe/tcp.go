package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

type TCPProbe struct {
	Target             string
	TLS                bool
	ServerName         string
	InsecureSkipVerify bool
	Assertions         []Assertion
}

func init() {
	Register("tcp", func(params map[string]any) (Probe, error) {
		target, _ := params["target"].(string)
		if target == "" {
			return nil, fmt.Errorf("tcp probe requires 'target' (host:port)")
		}
		isTLS, _ := params["tls"].(bool)
		serverName, _ := params["server_name"].(string)
		insecure, _ := params["insecure_skip_verify"].(bool)

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
				Target:   "connected",
				Operator: "equals",
				Value:    "true",
			})
		}

		return &TCPProbe{
			Target:             target,
			TLS:                isTLS,
			ServerName:         serverName,
			InsecureSkipVerify: insecure,
			Assertions:         assertions,
		}, nil
	})
}

func (p *TCPProbe) Execute(ctx context.Context) Result {
	start := time.Now()
	res := Result{Timestamp: start, Details: make(map[string]any)}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", p.Target)
	if err != nil {
		res.Success = false
		res.Latency = time.Since(start)
		res.Message = fmt.Sprintf("tcp dial failed: %v", err)
		return res
	}
	defer conn.Close()

	tcpLatency := time.Since(start)
	res.Details["tcp_latency_ms"] = tcpLatency.Milliseconds()

	var certExpiryDays int64 = -1
	if p.TLS {
		sName := p.ServerName
		if sName == "" {
			host, _, _ := net.SplitHostPort(p.Target)
			sName = host
		}
		tlsConfig := &tls.Config{
			ServerName:         sName,
			InsecureSkipVerify: p.InsecureSkipVerify,
		}
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			res.Success = false
			res.Latency = time.Since(start)
			res.Message = fmt.Sprintf("tls handshake failed: %v", err)
			return res
		}
		defer tlsConn.Close()

		state := tlsConn.ConnectionState()
		if len(state.PeerCertificates) > 0 {
			cert := state.PeerCertificates[0]
			certExpiryDays = int64(time.Until(cert.NotAfter).Hours() / 24)
			res.Details["cert_expiry_days"] = certExpiryDays
			res.Details["cert_subject"] = cert.Subject.CommonName
		}
	}

	totalLatency := time.Since(start)
	res.Latency = totalLatency

	assertCtx := &AssertionContext{
		Connected:      true,
		LatencyMs:      totalLatency.Milliseconds(),
		CertExpiryDays: certExpiryDays,
	}

	ok, failureReason := EvaluateAssertions(p.Assertions, assertCtx)
	res.Success = ok
	if !ok {
		res.Message = failureReason
	}
	return res
}
