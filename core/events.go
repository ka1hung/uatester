package core

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ka1hung/opcua/client"
)

// Event is an event or alarm as the tool shows it.
type Event struct {
	EventID       string    `json:"eventId"` // hex
	Type          string    `json:"type"`    // Alarm, Condition, Event, RefreshStart, RefreshEnd or the type's node id
	Source        string    `json:"source"`
	SourceName    string    `json:"sourceName"`
	Time          time.Time `json:"time"`
	Message       string    `json:"message"`
	Severity      uint16    `json:"severity"`
	ConditionID   string    `json:"conditionId,omitempty"`
	ConditionName string    `json:"conditionName,omitempty"`
	Retain        bool      `json:"retain"`
	Active        bool      `json:"active"`
	Acked         bool      `json:"acked"`
}

// IsCondition reports whether the event is about an alarm or another
// condition.
func (e Event) IsCondition() bool { return e.ConditionID != "" }

// FromClient converts a client event.
func FromClient(e client.Event) Event {
	out := Event{EventID: hex.EncodeToString(e.EventID), Source: e.Source, SourceName: e.SourceName, Time: e.Time,
		Message: e.Message, Severity: e.Severity, ConditionID: e.ConditionID, ConditionName: e.ConditionName,
		Retain: e.Retain, Active: e.Active, Acked: e.Acked}
	switch {
	case e.RefreshStart:
		out.Type = "RefreshStart"
	case e.RefreshEnd:
		out.Type = "RefreshEnd"
	case e.EventType == "i=2915" || e.EventType == "i=2955" || e.EventType == "i=9341" || e.EventType == "i=9482":
		out.Type = "Alarm" // AlarmConditionType and the common limit alarm types
	case e.IsCondition():
		out.Type = "Condition"
	case e.EventType == "i=2041":
		out.Type = "Event"
	default:
		out.Type = e.EventType
	}
	return out
}

// ToClient turns an event back into what client.Acknowledge needs.
func (e Event) ToClient() (client.Event, error) {
	id, err := hex.DecodeString(e.EventID)
	if err != nil {
		return client.Event{}, fmt.Errorf("event id %q: %w", e.EventID, err)
	}
	return client.Event{EventID: id, ConditionID: e.ConditionID, Source: e.Source, ConditionName: e.ConditionName}, nil
}
