package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClassifyRequestAndRoutes(t *testing.T) {
	const input = "explain '$HOME' and $(touch never)\nwith \"quotes\""
	for _, tc := range []struct{ name, body, want string }{
		{"shell", `{"answers":{"route":{"type":"choice","choice":"shell","probabilities":{"shell":0.95,"natural_language":0.04,"ambiguous":0.01}}}}`, "shell"},
		{"codex", `{"answers":{"route":{"type":"choice","choice":"natural_language","probabilities":{"shell":0.02,"natural_language":0.97,"ambiguous":0.01}}}}`, "codex"},
		{"low", `{"answers":{"route":{"type":"choice","choice":"natural_language","probabilities":{"shell":0.05,"natural_language":0.8,"ambiguous":0.15}}}}`, "hold"},
		{"ambiguous", `{"answers":{"route":{"type":"choice","choice":"ambiguous","probabilities":{"shell":0.1,"natural_language":0.1,"ambiguous":0.8}}}}`, "hold"},
		{"ambiguous high confidence", `{"answers":{"route":{"type":"choice","choice":"ambiguous","probabilities":{"shell":0.001,"natural_language":0.001,"ambiguous":0.998}}}}`, "hold"},
		{"tied", `{"answers":{"route":{"type":"choice","choice":"natural_language","probabilities":{"shell":0.5,"natural_language":0.5,"ambiguous":0}}}}`, "hold"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("bad request headers")
				}
				var req requestBody
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				if req.Model != defaultModel || req.State["line"] != input || req.State["shell"] != "zsh" {
					t.Errorf("bad request state: %#v", req)
				}
				q := req.Questions["route"]
				if q.Type != "choice" || !strings.Contains(q.Instructions, "find the largest files") || !strings.Contains(q.Criteria["shell"], "quoted prose") {
					t.Errorf("bad route question: %#v", q)
				}
				w.Write([]byte(tc.body))
			}))
			defer s.Close()
			got, err := (&classifier{client: s.Client(), endpoint: s.URL, model: defaultModel, minProb: .9}).classify(context.Background(), "secret", input)
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestMalformedResponsesAndProviderErrors(t *testing.T) {
	for _, body := range []string{`{}`, `{"answers":{"route":{"type":"noul"}}}`, `{"answers":{"route":{"type":"choice","choice":"nope","probabilities":{"shell":1,"natural_language":0,"ambiguous":0}}}}`, `{"answers":{"route":{"type":"choice","choice":"shell","probabilities":{"shell":null,"natural_language":0,"ambiguous":1}}}}`, `{"answers":{"route":{"type":"choice","choice":"shell","probabilities":{"shell":2,"natural_language":0,"ambiguous":0}}}}`, `{"answers":{"route":{"type":"choice","choice":"shell","probabilities":{"shell":0.6,"natural_language":0.6,"ambiguous":0}}}}`, `{"answers":{"route":{"type":"choice","choice":"shell","probabilities":{"shell":0.9,"natural_language":0.05,"ambiguous":0.02}}}}`} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		_, err := (&classifier{client: s.Client(), endpoint: s.URL, minProb: .9}).classify(context.Background(), "k", "x")
		s.Close()
		if err == nil {
			t.Errorf("body accepted: %s", body)
		}
	}
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "provider-secret-body", status) }))
		_, err := (&classifier{client: s.Client(), endpoint: s.URL, minProb: .9}).classify(context.Background(), "k", "x")
		s.Close()
		if err == nil || strings.Contains(err.Error(), "provider-secret-body") {
			t.Fatalf("unsafe provider error: %v", err)
		}
	}
}

func TestLimitsTimeoutCancellationRedirectAndLiteralInput(t *testing.T) {
	tooLarge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, strings.Repeat("x", maxResponse+1)) }))
	_, err := (&classifier{client: tooLarge.Client(), endpoint: tooLarge.URL, minProb: .9}).classify(context.Background(), "k", "x")
	tooLarge.Close()
	if err == nil {
		t.Fatal("expected bounded response error")
	}
	delayed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(100 * time.Millisecond) }))
	if _, err := (&classifier{client: &http.Client{Timeout: time.Millisecond}, endpoint: delayed.URL, minProb: .9}).classify(context.Background(), "k", "x"); err == nil {
		t.Fatal("expected timeout")
	}
	delayed.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&classifier{client: http.DefaultClient, endpoint: "http://127.0.0.1:1", minProb: .9}).classify(ctx, "k", "x"); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect followed") }))
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
	}))
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	_, err = (&classifier{client: client, endpoint: redirect.URL, minProb: .9}).classify(context.Background(), "k", "x")
	redirect.Close()
	redirectTarget.Close()
	if err == nil || !strings.Contains(err.Error(), "302") {
		t.Fatalf("expected redirect rejection, got %v", err)
	}
	got, err := readLine(strings.NewReader("echo '$HOME'\n"))
	if err != nil || got != "echo '$HOME'\n" {
		t.Fatalf("literal input lost: %q %v", got, err)
	}
	if _, err := readLine(strings.NewReader(strings.Repeat("x", maxInput+1))); err == nil {
		t.Fatal("expected input limit")
	}
	if _, err := readLine(strings.NewReader("\xff")); err == nil {
		t.Fatal("expected invalid UTF-8 error")
	}
	if got, err := (&classifier{}).classify(context.Background(), "", " \n\t"); err != nil || got != "shell" {
		t.Fatalf("empty input should need no client or key: %q %v", got, err)
	}
}
