package gocksnap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/h2non/gock"
)

type recordMocker struct {
	// disabler stores a disabler for thread safety checking current mock is disabled
	disabler *disabler

	// snapshot stores the current snapshot being recorded.
	snapshot *Snapshot

	// existingCall is an optional existing call that can be used to pre-fill the response.
	existingCall *Call

	// call stores the current call being recorded.
	call *Call
}

type disabler struct {
	// disabled stores if the current mock is disabled.
	disabled bool

	// mutex stores the disabler mutex for thread safety.
	mutex sync.RWMutex
}

func (d *disabler) isDisabled() bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	return d.disabled
}

func (d *disabler) Disable() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.disabled = true
}

func (g *Snapshot) newRecordMock(existingCall *Call) *recordMocker {
	return &recordMocker{
		disabler:     new(disabler),
		snapshot:     g,
		existingCall: existingCall,
	}
}

// Disable disables the current mock manually.
func (r *recordMocker) Disable() {
	r.disabler.Disable()
}

// Done returns true if the current mock is done recording
func (r *recordMocker) Done() bool {
	// prevent deadlock with m.mutex
	if r.disabler.isDisabled() {
		return true
	}

	return r.call != nil
}

// Request returns an empty Request, this is not used.
func (r *recordMocker) Request() *gock.Request {
	return &gock.Request{}
}

// Response returns the Response selected by the user in the UI.
func (r *recordMocker) Response() *gock.Response {
	if r.call == nil {
		return &gock.Response{}
	}

	body, err := json.Marshal(r.call.MockedCall.Body)
	if err != nil {
		body = []byte(fmt.Sprintf("Error marshalling response: %v", err))
	}

	return &gock.Response{
		StatusCode: r.call.Status,
		BodyBuffer: body,
	}
}

// Match will prompt the user to select the response for the current request.
func (r *recordMocker) Match(req *http.Request) (bool, error) {
	if r.disabler.isDisabled() {
		return false, nil
	}

	r.call = r.snapshot.promptCall(req, r.existingCall)

	return true, nil
}

// SetMatcher is a no-op for the record mocker.
func (r *recordMocker) SetMatcher(_ gock.Matcher) {
}

// AddMatcher is a no-op for the record mocker.
func (r *recordMocker) AddMatcher(_ gock.MatchFunc) {
}
