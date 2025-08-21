package gocksnap

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
)

type CallPrompt struct {
	// Name is the name of the call, used for identification in the UI.
	Name string `json:"name"`

	// Request is the HTTP request that is being recorded.
	Request Request `json:"request"`

	// ExistingResponse is an optional existing call that can be used to pre-fill the response.
	ExistingResponse *MockedCall `json:"existingResponse,omitempty"`

	// finalCall will contain the selected response for the pending call.
	finalCall chan *Call
}

// startPromptServer starts a simple HTTP server to prompt the user for the setup mocks.
func (g *Snapshot) startPromptServer() (string, error) {
	conn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	mux := http.NewServeMux()

	// Default route to serve the index HTML
	mux.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, indexHTML)
	})

	// SSE endpoint to notify about pending calls
	mux.HandleFunc("/events", func(writer http.ResponseWriter, req *http.Request) {
		flusher, ok := writer.(http.Flusher)
		if !ok {
			http.Error(writer, "streaming not supported", http.StatusInternalServerError)

			return
		}

		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		writer.Header().Set("Connection", "keep-alive")

		callChan := make(chan string, 1)

		g.mu.Lock()
		g.sseConns[callChan] = struct{}{}
		g.mu.Unlock()

		defer func() {
			g.mu.Lock()
			delete(g.sseConns, callChan)
			g.mu.Unlock()
		}()

		for {
			select {
			case <-req.Context().Done():
				return
			case msg := <-callChan:
				// send the message to the client (CallPrompt struct)
				fmt.Fprintf(writer, "data: %s\n\n", msg)
				flusher.Flush()
			}
		}
	})

	// get the current pending call
	mux.HandleFunc("/current", func(writer http.ResponseWriter, _ *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()

		if g.pending == nil {
			http.Error(writer, "no snapshot pending", http.StatusNotFound)

			return
		}

		err = json.NewEncoder(writer).Encode(g.pending)
		if err != nil {
			http.Error(writer, "failed to encode pending call", http.StatusInternalServerError)

			return
		}
	})

	// submit the response for the pending call
	mux.HandleFunc("/new", func(writer http.ResponseWriter, req *http.Request) {
		var payload MockedCall

		err = json.NewDecoder(req.Body).Decode(&payload)
		if err != nil {
			http.Error(writer, "Invalid JSON payload", http.StatusBadRequest)

			return
		}

		g.mu.Lock()

		if g.pending != nil {
			g.pending.finalCall <- &Call{
				Request:    g.pending.Request,
				MockedCall: payload,
			}

			g.pending = nil
		}

		g.mu.Unlock()

		fmt.Fprint(writer, "ok")
	})

	// reuse an existing response for the pending call
	mux.HandleFunc("/existing", func(writer http.ResponseWriter, req *http.Request) {
		var payload MockedCall

		err = json.NewDecoder(req.Body).Decode(&payload)
		if err != nil {
			http.Error(writer, "Invalid JSON payload", http.StatusBadRequest)

			return
		}

		g.mu.Lock()

		if g.pending == nil {
			http.Error(writer, "no snapshot pending", http.StatusNotFound)
			g.mu.Unlock()

			return
		}

		if g.pending.ExistingResponse == nil {
			http.Error(writer, "no existing response available", http.StatusNotFound)
			g.mu.Unlock()

			return
		}

		g.pending.finalCall <- &Call{
			Request:    g.pending.Request,
			MockedCall: payload,
		}
		g.pending = nil
		g.mu.Unlock()

		fmt.Fprint(writer, "ok")
	})

	go http.Serve(conn, mux)

	return "http://" + conn.Addr().String(), nil
}

// openBrowser opens the given URL in the default web browser, depending on the OS.
func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	_ = cmd.Start()
}
