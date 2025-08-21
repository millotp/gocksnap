package gocksnap

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/h2non/gock"
)

const defaultSnapshotDirectory = "__snapshots__"

//go:embed index.html
var indexHTML string

type Request struct {
	// Method is the HTTP method of the call (GET, POST, etc.)
	Method string `json:"method"`

	// URL is the full URL of the request.
	URL string `json:"url"`

	// Body is the request body, if any.
	Body json.RawMessage `json:"reqBody"`

	// Headers is the request headers, if any.
	Headers map[string][]string `json:"headers,omitempty"`

	// QueryParams is the query parameters of the request, if any.
	QueryParams map[string][]string `json:"queryParams,omitempty"`
}

type MockedCall struct {
	// Status is the HTTP status code of the response.
	Status int `json:"status"`
	// Body is the response body , if any.
	Body json.RawMessage `json:"resBody"`

	// MatchingHeaders is a map of headers that should match the request.
	MatchingHeaders map[string][]string `json:"matchingHeaders,omitempty"`

	// MatchingQueryParams is a map of query parameters that should match the request.
	MatchingQueryParams map[string][]string `json:"matchingQueryParams,omitempty"`
}

// Call represents a single HTTP call in the snapshot.
type Call struct {
	Request
	MockedCall
}

// Snapshot holds the state of the snapshot being recorded, which can include multiple HTTP calls.
type Snapshot struct {
	Calls []Call `json:"calls"`

	// testName used to identify the snapshot file.
	testName string

	// name is the name of the snapshot, used for identification in the test and in the file.
	name string

	// updateMode indicates if the snapshot is in update mode or in test mode.
	updateMode bool

	// mu is a mutex to protect access to the pending call and SSE connections.
	mu sync.Mutex

	// pending is the current call that is being recorded.
	pending *CallPrompt

	// sseConns is a map of client connections that are waiting for updates.
	sseConns map[chan string]struct{}
}

// Finish
func (g *Snapshot) Finish(t *testing.T) {
	t.Helper()

	if !g.updateMode {
		if !gock.IsDone() {
			t.Fatalf("Snapshot '%s' is not complete. Some requests were not mocked.", g.name)
		}

		return
	}

	// Update snapshot
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal snapshot '%s': %v", g.name, err)
	}

	_ = os.MkdirAll(defaultSnapshotDirectory, 0o750)

	err = os.WriteFile(g.file(), data, 0o600)
	if err != nil {
		t.Fatalf("Failed to save snapshot '%s': %v", g.name, err)
	}
}

// file returns the path to the snapshot file.
func (g *Snapshot) file() string {
	return filepath.Join(defaultSnapshotDirectory, strings.ReplaceAll(strings.ReplaceAll(g.testName+"-"+g.name, " ", "_"), "/", "_")+".json")
}

// promptCall sends the current request to the UI for user interaction.
func (g *Snapshot) promptCall(req *http.Request, existingCall *Call) *Call {
	var bodyRaw []byte
	if req.Body != nil {
		bodyRaw, _ = io.ReadAll(req.Body)
	}

	g.mu.Lock()

	fmt.Printf("Request: %s %s\n", req.Method, req.URL.String())

	queryParams := req.URL.Query()
	urlWithoutQuery := req.URL
	urlWithoutQuery.RawQuery = ""

	finalCall := make(chan *Call, 1)

	g.pending = &CallPrompt{
		Name: g.name,
		Request: Request{
			Method:      req.Method,
			URL:         urlWithoutQuery.String(),
			Body:        bodyRaw,
			Headers:     req.Header,
			QueryParams: queryParams,
		},
		ExistingResponse: &existingCall.MockedCall,
		finalCall:        finalCall,
	}

	// notify SSE clients
	for ch := range g.sseConns {
		select {
		case ch <- "pending":
		default:
		}
	}

	g.mu.Unlock()

	return <-finalCall
}

// MatchSnapshot creates a new snapshot for the current test.
// If the snapshot file is not found, or if the environment variable UPDATE_GOCKSNAP is set to "true", it will spawn a web server to allow the user to interactively select responses for the recorded requests.
// If the snapshot file is found, it will load the existing calls and register them with gock.
// After all the calls are finished, the user should call the Finish method to save the snapshot / assert that all calls were mocked correctly.
func MatchSnapshot(t *testing.T, snapshotName string) *Snapshot {
	t.Helper()

	snapshot := &Snapshot{
		Calls:      []Call{},
		testName:   t.Name(),
		name:       snapshotName,
		updateMode: os.Getenv("UPDATE_GOCKSNAP") == "true",
		sseConns:   make(map[chan string]struct{}),
	}

	var existingCalls []Call

	_, err := os.Stat(snapshot.file())
	if os.IsNotExist(err) {
		// can't find snapshot file, so we are in update mode
		t.Logf("Snapshot '%s' not found, running in update mode\n", snapshot.file())
		snapshot.updateMode = true
	} else {
		// Load existing snapshot
		data, err := os.ReadFile(snapshot.file())
		if err != nil {
			t.Fatalf("Failed to open snapshot '%s': %v", snapshot.file(), err)
		}

		err = json.Unmarshal(data, snapshot)
		if err != nil {
			t.Fatalf("Failed to unmarshal snapshot '%s': %v", snapshot.file(), err)
		}

		if snapshot.updateMode {
			existingCalls = snapshot.Calls
			snapshot.Calls = make([]Call, 0)
			t.Logf("Updating existing snapshot '%s'\n", snapshot.file())
		}
	}

	if snapshot.updateMode {
		addr, err := snapshot.startPromptServer()
		if err != nil {
			t.Fatalf("Failed to start prompt server for snapshot '%s': %v", snapshot.file(), err)
		}

		openBrowser(addr)
	}

	gock.Intercept()

	if snapshot.updateMode {
		var existingCall *Call
		if len(existingCalls) > 0 {
			existingCall = &existingCalls[0]
		}

		// clean up any existing mocks
		for _, mock := range gock.Pending() {
			mock.Disable()
		}

		gock.Register(snapshot.newRecordMock(existingCall))
		gock.Observe(func(_ *http.Request, mock gock.Mock) {
			snapshot.Calls = append(snapshot.Calls, *mock.(*recordMocker).call)
			// load the next one
			existingCall = nil
			if len(snapshot.Calls) < len(existingCalls) {
				existingCall = &existingCalls[len(snapshot.Calls)]
			}

			gock.Register(snapshot.newRecordMock(existingCall))
		})

		return snapshot
	}

	// Register existing calls into gock.
	for _, call := range snapshot.Calls {
		req := gock.NewRequest().URL(call.URL).JSON(call.Request.Body)
		req.Method = strings.ToUpper(call.Method)
		for key, values := range call.MockedCall.MatchingHeaders {
			req.MatchHeader(key, strings.Join(values, ","))
		}
		for key, values := range call.MockedCall.MatchingQueryParams {
			req.MatchParam(key, strings.Join(values, ","))
		}
		gock.Register(gock.NewMock(req, gock.NewResponse().Status(call.Status).JSON(call.MockedCall.Body)))
	}

	t.Logf("Loaded snapshot '%s' with %d calls", snapshot.file(), len(snapshot.Calls))

	return snapshot
}
