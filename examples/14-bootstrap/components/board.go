package components

import (
	"slices"
	"time"

	"github.com/nkapatos/templatic/live"
)

// Statuses are the states a service can be in, in button-group order.
var Statuses = []string{"up", "degraded", "down"}

// Service is one card on the board.
type Service struct {
	ID, Name string
	Status   string
	// Since is when Status last changed.
	Since time.Time
	// LatencyMS is the last measured latency.
	LatencyMS int
	// Incident is the note of the open incident, "" when there is none.
	Incident string
}

// SetIn is the input of Set: a service and its new status.
type SetIn struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

// Validate refuses an unknown status; the store refuses an unknown service.
func (in SetIn) Validate() error {
	if !slices.Contains(Statuses, in.Status) {
		return live.Invalid("status", "pick up, degraded or down")
	}
	return nil
}

// ReportIn is the input of Report: which service is affected and how.
type ReportIn struct {
	Service string `json:"service"`
	Note    string `json:"note"`
}

// Validate refuses an empty or overlong note.
func (in ReportIn) Validate() error {
	if in.Service == "" {
		return live.Invalid("service", "choose a service")
	}
	if n := len([]rune(in.Note)); n < 3 || n > 80 {
		return live.Invalid("note", "describe it in 3 to 80 characters")
	}
	return nil
}

// Set changes a service's status.
var Set = live.NewCommand[SetIn]("service.set")

// Report opens an incident on a service and marks it down.
var Report = live.NewCommand[ReportIn]("incident.report")

func badgeClass(status string) string {
	switch status {
	case "up":
		return "text-bg-success"
	case "degraded":
		return "text-bg-warning"
	default:
		return "text-bg-danger"
	}
}
