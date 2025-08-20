package gocksnap

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/h2non/gock"
)

func TestSnapshot(t *testing.T) {
	defer gock.Off()

	snapshot := MatchSnapshot(t, "works with multiple calls")

	c := &http.Client{}
	resp, err := c.Post("https://test.com", "application/json", strings.NewReader(`{"req1": "value"}`))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp == nil {
		t.Fatal("Response is nil")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if strings.TrimSpace(string(body)) != `{"res1":"data"}` {
		t.Fatalf("Unexpected response body: '%s'", body)
	}

	resp, err = c.Post("https://example.com", "application/json", strings.NewReader(`{"req2": "value"}`))
	if err != nil {
		t.Fatalf("Failed to make second request: %v", err)
	}

	if resp == nil {
		t.Fatal("Response is nil for second request")
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read second response body: %v", err)
	}
	if strings.TrimSpace(string(body)) != `{"res2":"data"}` {
		t.Fatalf("Unexpected second response body: %s", body)
	}

	snapshot.Finish(t)

	snapshot = MatchSnapshot(t, "works with a second scenario")

	resp, err = c.Post("https://other.com", "application/json", strings.NewReader(`{"req3": "value"}`))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	if resp == nil {
		t.Fatal("Response is nil")
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if strings.TrimSpace(string(body)) != `{"res3":"data"}` {
		t.Fatalf("Unexpected response body: '%s'", body)
	}

	snapshot.Finish(t)
}
