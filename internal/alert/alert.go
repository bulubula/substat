package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type EventType string

const (
	EventDown      EventType = "DOWN"
	EventRecovered EventType = "RECOVERED"
)

type Event struct {
	MonitorName string        `json:"monitor_name"`
	EventType   EventType     `json:"event_type"`
	Timestamp   time.Time     `json:"timestamp"`
	Latency     time.Duration `json:"latency"`
	Message     string        `json:"message"`
	FailCount   int           `json:"fail_count"`
}

type Notifier interface {
	Send(ctx context.Context, event Event) error
}

type Factory func(params map[string]any) (Notifier, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

func Register(alertType string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[alertType] = factory
}

func Create(alertType string, params map[string]any) (Notifier, error) {
	registryMu.RLock()
	factory, ok := registry[alertType]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown alert notifier type %q", alertType)
	}
	return factory(params)
}

// Built-in Bark Notifier
type BarkNotifier struct {
	BaseURL string
	Group   string
}

func init() {
	Register("bark", func(params map[string]any) (Notifier, error) {
		u, _ := params["url"].(string)
		if u == "" {
			return nil, fmt.Errorf("bark notifier requires 'url'")
		}
		group, _ := params["group"].(string)
		if group == "" {
			group = "Substat"
		}
		return &BarkNotifier{BaseURL: u, Group: group}, nil
	})

	Register("webhook", func(params map[string]any) (Notifier, error) {
		u, _ := params["url"].(string)
		if u == "" {
			return nil, fmt.Errorf("webhook notifier requires 'url'")
		}
		method, _ := params["method"].(string)
		if method == "" {
			method = "POST"
		}
		headers := make(map[string]string)
		if rawHeaders, ok := params["headers"].(map[string]any); ok {
			for k, v := range rawHeaders {
				headers[k] = fmt.Sprint(v)
			}
		}
		return &WebhookNotifier{URL: u, Method: method, Headers: headers}, nil
	})
}

func (b *BarkNotifier) Send(ctx context.Context, e Event) error {
	title := fmt.Sprintf("[%s] %s", e.EventType, e.MonitorName)
	body := fmt.Sprintf("Time: %s\nLatency: %v\nInfo: %s",
		e.Timestamp.Format("2006-01-02 15:04:05"),
		e.Latency.Round(time.Millisecond),
		e.Message,
	)

	reqURL := fmt.Sprintf("%s/%s/%s?group=%s",
		b.BaseURL,
		url.PathEscape(title),
		url.PathEscape(body),
		url.QueryEscape(b.Group),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// Built-in Webhook Notifier
type WebhookNotifier struct {
	URL     string
	Method  string
	Headers map[string]string
}

func (w *WebhookNotifier) Send(ctx context.Context, e Event) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, w.Method, w.URL, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	for k, v := range w.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
