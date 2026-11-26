package live

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultTokenTTL bounds how old a page token may be at connect time.
const DefaultTokenTTL = 12 * time.Hour

// WithKey sets the HMAC key that signs page tokens.
func WithKey(key []byte) Option {
	return func(w *World) { w.key = append([]byte(nil), key...) }
}

// WithTokenTTL sets how long a page token stays valid (default DefaultTokenTTL).
func WithTokenTTL(d time.Duration) Option {
	return func(w *World) {
		if d > 0 {
			w.tokenTTL = d
		}
	}
}

// TokenOption customizes a page token.
type TokenOption func(*tokenClaims)

// TokenParam attaches an attested key/value to the token.
func TokenParam(key, value string) TokenOption {
	return func(c *tokenClaims) {
		if c.Params == nil {
			c.Params = map[string]string{}
		}
		c.Params[key] = value
	}
}

// Reserved param keys PageToken fills from the request the SSR saw.
const (
	ParamPath  = "tc.path"  // r.URL.Path at render
	ParamQuery = "tc.query" // r.URL.RawQuery at render
)

// SeedCell pre-declares a session cell on the session the token creates.
func SeedCell(topic string, value any) TokenOption {
	validateTopic(topic)
	raw, err := json.Marshal(value, json.Deterministic(true))
	if err != nil {
		panic(fmt.Errorf("live: SeedCell %q value is not JSON-encodable: %w", topic, err))
	}
	return func(c *tokenClaims) {
		if c.Seeds == nil {
			c.Seeds = map[string]jsontext.Value{}
		}
		c.Seeds[topic] = raw
	}
}

type tokenClaims struct {
	SID    string                    `json:"s"`
	Issued int64                     `json:"i"`
	Params map[string]string         `json:"p,omitempty"`
	Seeds  map[string]jsontext.Value `json:"c,omitempty"`
}

// PageToken mints a session identity and returns the signed token that proves it.
func (w *World) PageToken(r *http.Request, opts ...TokenOption) string {
	claims := tokenClaims{SID: mintSID(), Issued: w.clock().Unix()}
	if r != nil && r.URL != nil {
		claims.Params = map[string]string{ParamPath: r.URL.Path, ParamQuery: r.URL.RawQuery}
	}
	for _, opt := range opts {
		opt(&claims)
	}
	payload, err := json.Marshal(claims, json.Deterministic(true))
	if err != nil {
		// Cannot fail for these types; fail closed anyway.
		return ""
	}
	return w.sign(payload)
}

var (
	errTokenFormat  = errors.New("live: malformed page token")
	errTokenForged  = errors.New("live: page token signature invalid")
	errTokenExpired = errors.New("live: page token expired")
)

func (w *World) verifyToken(tok string) (tokenClaims, error) {
	var claims tokenClaims
	body, sig, ok := strings.Cut(tok, ".")
	if !ok || body == "" || sig == "" {
		return claims, errTokenFormat
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return claims, errTokenFormat
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return claims, errTokenFormat
	}
	mac := hmac.New(sha256.New, w.signingKey())
	mac.Write(payload)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return claims, errTokenForged
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.SID == "" {
		return claims, errTokenFormat
	}
	issued := time.Unix(claims.Issued, 0)
	now := w.clock()
	if now.Sub(issued) > w.tokenTTL {
		return claims, errTokenExpired
	}
	if issued.Sub(now) > time.Minute {
		return claims, errTokenFormat
	}
	return claims, nil
}

func (w *World) sign(payload []byte) string {
	mac := hmac.New(sha256.New, w.signingKey())
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (w *World) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

var devKeyWarn sync.Once

func (w *World) signingKey() []byte {
	if len(w.key) > 0 {
		return w.key
	}
	w.devKeyOnce.Do(func() {
		w.devKey = make([]byte, 32)
		if _, err := rand.Read(w.devKey); err != nil {
			panic(err) // no entropy: nothing safe to do
		}
		devKeyWarn.Do(func() {
			log.Printf("live: no WithKey configured; signing page tokens with a per-process DEVELOPMENT key " +
				"(tokens die with this process and are not shared across replicas). Set live.WithKey in production.")
		})
	})
	return w.devKey
}

func mintSID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Constant-time comparison.
func tokensEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
