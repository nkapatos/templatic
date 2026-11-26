package live

import "encoding/json/jsontext"

// Wire event kinds.
const (
	KindCell   = "cell"
	KindSwap   = "swap"
	KindAppend = "append"
	KindReady  = "ready"
	KindResync = "resync"
	KindDev    = "dev"
)

// version orders value-shaped events of one topic when offers interleave.
type event struct {
	kind    string
	topic   string
	body    jsontext.Value // marshaled {"v":…} or {"html":"…"}
	version uint64
}

// Keys follow the topic grammar because the client uses them in query selectors.
type streamItem struct {
	K    string `json:"k"`
	HTML string `json:"html"`
}

type commandRequest struct {
	SID    string         `json:"sid"`
	Action string         `json:"action"`
	Args   jsontext.Value `json:"args"`
	Form   *string        `json:"form"`
	Submit string         `json:"submit,omitempty"`
}
