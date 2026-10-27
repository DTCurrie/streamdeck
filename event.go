package streamdeck

import (
	"encoding/binary"
	"fmt"
)

const DialMax = 100

type EventKind int

const (
	EventUnknown = iota
	EventKeyPressed
	EventKeyReleased
	EventDialPressed
	EventDialReleased
	EventDialTurn
	// EventTouchTap is a short touch on the touch strip (Stream Deck +).
	EventTouchTap
	// EventTouchLongPress is a long touch on the touch strip (Stream Deck +).
	EventTouchLongPress
	// EventTouchSwipe is a drag across the touch strip (Stream Deck +).
	EventTouchSwipe
)

func (ev EventKind) String() string {
	switch ev {
	case EventKeyPressed:
		return "key-pressed"
	case EventKeyReleased:
		return "key-released"
	case EventDialPressed:
		return "dial-pressed"
	case EventDialReleased:
		return "dial-released"
	case EventDialTurn:
		return "dial-turn"
	case EventTouchTap:
		return "touch-tap"
	case EventTouchLongPress:
		return "touch-long-press"
	case EventTouchSwipe:
		return "touch-swipe"
	default:
		return "unknown"
	}
}

// Event is one input event. Which is the key or dial index; it is 0 for touch events.
type Event struct {
	Kind  EventKind
	Which int
	// Delta is the number of detents a dial turned, positive clockwise. Set for EventDialTurn.
	Delta int
	// X and Y are the touch position on the touch strip in pixels. Set for the touch events.
	X, Y int
	// ToX and ToY are where the deck recognised a swipe, about 50 px from X, Y in the direction of
	// travel; the deck reports the swipe then, not when the finger lifts. Set for EventTouchSwipe.
	ToX, ToY int
}

func (e Event) String() string {
	switch e.Kind {
	case EventTouchTap, EventTouchLongPress:
		return fmt.Sprintf("%s:%d,%d", e.Kind.String(), e.X, e.Y)
	case EventTouchSwipe:
		return fmt.Sprintf("%s:%d,%d->%d,%d", e.Kind.String(), e.X, e.Y, e.ToX, e.ToY)
	default:
		return fmt.Sprintf("%s:%d", e.Kind.String(), e.Which)
	}
}

type State struct {
	Keys     []bool
	DialPush []bool
	DialPos  []int // 0 -> 100
}

func (s *State) Update(c *Config, b []byte) ([]Event, error) {
	if b[0] != 1 {
		return nil, fmt.Errorf("why isn't it starting with 1, %v", b)
	}

	// see https://github.com/dh1tw/streamdeck/pull/9#discussion_r2187628307
	if c != nil && c.ConvertKey {
		return s.updateKeyPressOriginal(b[1:])
	}

	switch b[1] {
	case 0:
		return s.updateKeyPress(b[4:])
	case 2:
		return updateTouch(b)
	case 3:
		if b[4] == 0 {
			return s.updateDialPush(b[5:])
		}
		return s.updateDialTurn(b[5:])
	default:
		return nil, fmt.Errorf("unknown event type %d", b[1])
	}
}

func applyBools(in []bool, data []byte) ([]int, []bool) {
	changed := []int{}
	for i, b := range data {
		if len(in) <= i {
			in = append(in, false)
		}

		prev := in[i]

		if b == 0 {
			in[i] = false
		} else {
			in[i] = true
		}

		if prev != in[i] {
			changed = append(changed, i)
		}
	}
	return changed, in
}

func (s *State) updateKeyPressOriginal(data []byte) ([]Event, error) {
	changedKeys := []int{}
	updateEvents := []Event{}

	if len(data) < 15 {
		return nil, fmt.Errorf("wrong amount of data for updateKeyPressOriginal %d", len(data))
	}
	nd := make([]byte, 15)
	for x := 0; x < 3; x++ {
		for y := 0; y < 5; y++ {
			nd[(x*5)+y] = data[(x*5)+4-y]
		}
	}

	changedKeys, s.Keys = applyBools(s.Keys, nd)
	for _, changedKey := range changedKeys {
		if s.Keys[changedKey] {
			updateEvents = append(updateEvents, Event{Kind: EventKeyPressed, Which: changedKey})
		} else {
			updateEvents = append(updateEvents, Event{Kind: EventKeyReleased, Which: changedKey})
		}
	}
	return updateEvents, nil
}

func (s *State) updateKeyPress(data []byte) ([]Event, error) {
	changedKeys := []int{}
	updateEvents := []Event{}

	changedKeys, s.Keys = applyBools(s.Keys, data)
	for _, changedKey := range changedKeys {
		if s.Keys[changedKey] {
			updateEvents = append(updateEvents, Event{Kind: EventKeyPressed, Which: changedKey})
		} else {
			updateEvents = append(updateEvents, Event{Kind: EventKeyReleased, Which: changedKey})
		}
	}
	return updateEvents, nil
}

func (s *State) updateDialPush(data []byte) ([]Event, error) {
	var changedDialPushs []int
	changedDialPushs, s.DialPush = applyBools(s.DialPush, data)

	updateEvents := []Event{}
	for _, changedKey := range changedDialPushs {
		if s.DialPush[changedKey] {
			updateEvents = append(updateEvents, Event{Kind: EventDialPressed, Which: changedKey})
		} else {
			updateEvents = append(updateEvents, Event{Kind: EventDialReleased, Which: changedKey})
		}
	}
	return updateEvents, nil
}

// updateDialTurn emits one event per dial that moved, carrying its signed delta, and keeps
// DialPos (clamped to 0..DialMax) for callers that want an absolute position.
func (s *State) updateDialTurn(data []byte) ([]Event, error) {
	updateEvents := []Event{}

	for i, d := range data {
		if len(s.DialPos) <= i {
			s.DialPos = append(s.DialPos, DialMax/2)
		}

		delta := int(int8(d))
		if delta == 0 {
			continue
		}

		s.DialPos[i] = max(0, min(DialMax, s.DialPos[i]+delta))
		updateEvents = append(updateEvents, Event{Kind: EventDialTurn, Which: i, Delta: delta})
	}

	if len(updateEvents) == 0 {
		return nil, nil
	}
	return updateEvents, nil
}

// touchReportLen is the shortest touch report: id, type, two reserved bytes, the event type,
// one reserved byte, and four little-endian uint16 coordinates.
const touchReportLen = 14

const (
	touchTypeTap       = 1
	touchTypeLongPress = 2
	touchTypeSwipe     = 3
)

func updateTouch(b []byte) ([]Event, error) {
	if len(b) < touchReportLen {
		return nil, fmt.Errorf("touch report too short: %d bytes", len(b))
	}

	e := Event{
		X: int(binary.LittleEndian.Uint16(b[6:])),
		Y: int(binary.LittleEndian.Uint16(b[8:])),
	}

	switch b[4] {
	case touchTypeTap:
		e.Kind = EventTouchTap
	case touchTypeLongPress:
		e.Kind = EventTouchLongPress
	case touchTypeSwipe:
		e.Kind = EventTouchSwipe
		e.ToX = int(binary.LittleEndian.Uint16(b[10:]))
		e.ToY = int(binary.LittleEndian.Uint16(b[12:]))
	default:
		return nil, fmt.Errorf("unknown touch event type %d", b[4])
	}

	return []Event{e}, nil
}
