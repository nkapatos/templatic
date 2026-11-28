package live

import "encoding/json/jsontext"

// Wire event kinds.
const (
	KindCell   = "cell"
	KindSwap   = "swap"
	KindOps    = "ops"
	KindReset  = "reset"
	KindReady  = "ready"
	KindResync = "resync"
	KindToken  = "token"
	KindDev    = "dev"
)

// Resync reasons: why a page's session cannot go on.
const (
	ReasonLost     = "lost"     // the server no longer holds the session, or cannot catch it up
	ReasonOverflow = "overflow" // the page fell too far behind
	ReasonRevoked  = "revoked"  // the app ended it (World.EndSessions)
	ReasonMaxAge   = "max-age"  // older than WithMaxSessionAge
	ReasonRefused  = "refused"  // the app's WithAdmit refused the connect
)

func resyncBody(reason string) jsontext.Value {
	return jsontext.Value(`{"reason":"` + reason + `"}`)
}

// version orders value-shaped events of one topic when offers interleave.
type event struct {
	kind    string
	topic   string
	body    jsontext.Value // marshaled {"v":…} or {"html":"…"}
	version uint64
}

type commandRequest struct {
	SID    string         `json:"sid"`
	Action string         `json:"action"`
	Args   jsontext.Value `json:"args"`
	Form   *string        `json:"form"`
	Submit string         `json:"submit,omitempty"`
	// Subscribe, sent with no action, maps collection topics to the version the page holds.
	Subscribe map[string]int64 `json:"subscribe,omitempty"`
}
