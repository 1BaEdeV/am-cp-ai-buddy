package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTPAgentReply(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var request AgentRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.TaskID != "team-intro" || request.Question != "Кто наставник?" || request.Context == nil {
			t.Fatalf("unexpected ML request: %#v", request)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"text":"Твой наставник — Катя."}`)), Header: make(http.Header)}, nil
	})}

	agent := NewHTTPAgent("http://ml.test/chat", client)
	answer, err := agent.Reply(context.Background(), AgentRequest{
		TaskID: "team-intro", Question: "Кто наставник?", Context: map[string]string{"mentor": "Катя"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Text != "Твой наставник — Катя." {
		t.Fatalf("unexpected answer: %q", answer.Text)
	}
}

func TestHTTPAgentRejectsServiceErrorsAndEmptyAnswers(t *testing.T) {
	for _, test := range []struct {
		name string
		code int
		body string
	}{
		{name: "service unavailable", code: http.StatusServiceUnavailable, body: `{"error":"agent_unavailable"}`},
		{name: "empty answer", code: http.StatusOK, body: `{"text":"  "}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.code, Status: "test", Body: io.NopCloser(strings.NewReader(test.body)), Header: make(http.Header)}, nil
			})}
			agent := NewHTTPAgent("http://ml.test/chat", client)
			_, err := agent.Reply(context.Background(), AgentRequest{TaskID: "team-intro", Question: "Q"})
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
