package live

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SubmitHeader carries a fresh submission id on a form command's response.
const SubmitHeader = "X-TC-Submit"

// The domain prefix keeps a submission id from verifying as a page token.
const submitDomain = "tc-submit\x00"

type submitClaims struct {
	SID    string `json:"s"`
	Nonce  string `json:"n"`
	Issued int64  `json:"i"`
}

// submission is one claimed id; done closes when its first run settles.
type submission struct {
	done     chan struct{}
	ok       bool
	js       bool
	status   int
	ctype    string
	body     []byte
	redirect string
}

type submitStore struct {
	mu    sync.Mutex
	m     map[string]*submitEntry
	swept time.Time
}

type submitEntry struct {
	s       *submission
	expires time.Time
}

// SubmitID mints a submission id for the session pageToken names, or "" when
// the token does not verify.
func (w *World) SubmitID(pageToken string) string {
	c, err := w.verifyToken(pageToken)
	if err != nil {
		return ""
	}
	return w.mintSubmit(c.SID)
}

func (w *World) mintSubmit(sid string) string {
	payload, err := json.Marshal(submitClaims{SID: sid, Nonce: mintSID(), Issued: w.clock().Unix()}, json.Deterministic(true))
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(w.submitMAC(payload))
}

func (w *World) submitMAC(payload []byte) []byte {
	mac := hmac.New(sha256.New, w.signingKey())
	mac.Write([]byte(submitDomain))
	mac.Write(payload)
	return mac.Sum(nil)
}

func (w *World) verifySubmit(id string) (submitClaims, bool) {
	var c submitClaims
	body, sig, ok := strings.Cut(id, ".")
	if !ok {
		return c, false
	}
	payload, err1 := base64.RawURLEncoding.DecodeString(body)
	got, err2 := base64.RawURLEncoding.DecodeString(sig)
	if err1 != nil || err2 != nil || !hmac.Equal(got, w.submitMAC(payload)) {
		return c, false
	}
	if json.Unmarshal(payload, &c) != nil || c.SID == "" || c.Nonce == "" {
		return c, false
	}
	if w.clock().Sub(time.Unix(c.Issued, 0)) > w.tokenTTL {
		return c, false
	}
	return c, true
}

// claimSubmit claims a verified id. The first caller runs (first is true) and
// must call finish; a repeat waits for the first run and gets its settled
// submission, or runs itself when that run failed.
func (w *World) claimSubmit(c submitClaims) (s *submission, first bool) {
	st := &w.submits
	expires := time.Unix(c.Issued, 0).Add(w.tokenTTL)
	for {
		st.mu.Lock()
		now := w.clock()
		if now.Sub(st.swept) > time.Minute {
			for k, e := range st.m {
				if now.After(e.expires) {
					delete(st.m, k)
				}
			}
			st.swept = now
		}
		e := st.m[c.Nonce]
		if e == nil || e.s.settledFailed() {
			if st.m == nil {
				st.m = map[string]*submitEntry{}
			}
			s = &submission{done: make(chan struct{})}
			st.m[c.Nonce] = &submitEntry{s: s, expires: expires}
			st.mu.Unlock()
			return s, true
		}
		st.mu.Unlock()
		<-e.s.done
		if e.s.ok {
			return e.s, false
		}
	}
}

func (s *submission) settledFailed() bool {
	select {
	case <-s.done:
		return !s.ok
	default:
		return false
	}
}

// settle records the first run's outcome and releases waiting repeats.
func (s *submission) settle(ok bool) {
	s.ok = ok
	close(s.done)
}

// responseTee records what a handler writes so a repeat can be answered with it.
type responseTee struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (t *responseTee) WriteHeader(code int) {
	if t.status == 0 {
		t.status = code
	}
	t.ResponseWriter.WriteHeader(code)
}

func (t *responseTee) Write(b []byte) (int, error) {
	if t.status == 0 {
		t.status = http.StatusOK
	}
	t.body.Write(b)
	return t.ResponseWriter.Write(b)
}

// replayJS answers a repeated command with the first run's response.
func (s *submission) replayJS(w http.ResponseWriter) {
	if !s.js || s.status == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.ctype != "" {
		w.Header().Set("Content-Type", s.ctype)
	}
	w.WriteHeader(s.status)
	_, _ = w.Write(s.body)
}

// replayForm answers a repeated no-JS POST: the first POST's redirect, or the
// submitting page when the first run came from the runtime.
func (s *submission) replayForm(w http.ResponseWriter, r *http.Request) {
	if !s.js {
		http.Redirect(w, r, s.redirect, http.StatusSeeOther)
		return
	}
	if ref, err := url.Parse(r.Referer()); err == nil && ref.Host == r.Host && ref.Path != "" {
		http.Redirect(w, r, ref.RequestURI(), http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
