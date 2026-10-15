package messaging

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ChatChanged is the data of "chat" events.
type ChatChanged struct {
	ChatID string `json:"chat_id"`
}

// TypingChanged is the data of "typing" events.
type TypingChanged struct {
	ChatID   string `json:"chat_id"`
	PersonID string `json:"person_id"`
	Typing   bool   `json:"typing"`
}

// StreamEvent is one server-sent event: Name is one of the Event* constants, Data its JSON.
type StreamEvent struct {
	Name string
	Data json.RawMessage
}

// WriteEvent writes one server-sent event, for providers.
func WriteEvent(w io.Writer, name string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
	return err
}

// Events streams the provider's server-sent events until ctx ends or the connection drops; then the channel is
// closed. Reconnect and treat the next "hello" as "anything may have changed".
func (p *Provider) Events(ctx context.Context) (<-chan StreamEvent, error) {
	req, err := p.c.request(ctx, http.MethodGet, p.url("v1/events", nil), p.AppID, nil, "")
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	// The stream outlives the client's request timeout.
	resp, err := (&http.Client{Transport: p.c.HTTP.Transport}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		return nil, decodeError(resp)
	}
	out := make(chan StreamEvent, 64)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64<<10), 1<<20)
		var name string
		var data strings.Builder
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				if name != "" {
					select {
					case out <- StreamEvent{Name: name, Data: json.RawMessage(data.String())}:
					case <-ctx.Done():
						return
					}
				}
				name = ""
				data.Reset()
			case strings.HasPrefix(line, ":"):
			case strings.HasPrefix(line, "event:"):
				name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
	}()
	return out, nil
}
