package live

import "encoding/json/v2"

// DevFrame is a dev-loop notice (wire kind "dev") sent when the build changes.
type DevFrame struct {
	What    string            `json:"what"`
	CSS     map[string]string `json:"css,omitempty"`
	Message string            `json:"message,omitempty"`
}

// Dev sends a dev-loop notice to every session; do not call it in production.
func (w *World) Dev(f DevFrame) {
	body, err := json.Marshal(f)
	if err != nil {
		return
	}
	w.publish(event{kind: KindDev, body: body})
}
