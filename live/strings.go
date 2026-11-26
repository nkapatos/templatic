package live

import (
	"encoding/json/v2"
	"fmt"
)

// Strings decodes a JSON string, an array of strings or null into a slice.
type Strings []string

// UnmarshalJSON accepts a JSON string, an array of strings, or null.
func (s *Strings) UnmarshalJSON(b []byte) error {
	// Check null first: null decodes into a string without error, as "", which
	// would become []string{""}.
	if string(b) == "null" {
		*s = nil
		return nil
	}
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*s = Strings{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("live: Strings wants a string or an array of strings, got %s", b)
	}
	*s = Strings(many)
	return nil
}
