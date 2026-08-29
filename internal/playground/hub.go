package playground

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

const maxDestLog = 50

type LogEvent struct {
	TS          string `json:"ts"`
	Line        string `json:"line"`
	DeliveryID  string `json:"delivery_id,omitempty"`
	BodyPreview string `json:"body_preview,omitempty"`
}

type sessionHub struct {
	mu       sync.RWMutex
	destLog  []LogEvent
	simSubs  map[chan LogEvent]struct{}
	destSubs map[chan LogEvent]struct{}
}

type Hub struct {
	mu       sync.RWMutex
	sessions map[uuid.UUID]*sessionHub
}

func NewHub() *Hub {
	return &Hub{sessions: make(map[uuid.UUID]*sessionHub)}
}

func (h *Hub) get(sessionID uuid.UUID) *sessionHub {
	h.mu.RLock()
	sh, ok := h.sessions[sessionID]
	h.mu.RUnlock()
	if ok {
		return sh
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if sh, ok = h.sessions[sessionID]; ok {
		return sh
	}
	sh = &sessionHub{
		simSubs:  make(map[chan LogEvent]struct{}),
		destSubs: make(map[chan LogEvent]struct{}),
	}
	h.sessions[sessionID] = sh
	return sh
}

func (h *Hub) SubscribeSim(sessionID uuid.UUID) (chan LogEvent, func()) {
	return h.subscribe(sessionID, true)
}

func (h *Hub) SubscribeDest(sessionID uuid.UUID) (chan LogEvent, func()) {
	return h.subscribe(sessionID, false)
}

func (h *Hub) subscribe(sessionID uuid.UUID, sim bool) (chan LogEvent, func()) {
	sh := h.get(sessionID)
	ch := make(chan LogEvent, 32)
	sh.mu.Lock()
	if sim {
		sh.simSubs[ch] = struct{}{}
	} else {
		sh.destSubs[ch] = struct{}{}
		for _, e := range sh.destLog {
			select {
			case ch <- e:
			default:
			}
		}
	}
	sh.mu.Unlock()
	return ch, func() {
		sh.mu.Lock()
		if sim {
			delete(sh.simSubs, ch)
		} else {
			delete(sh.destSubs, ch)
		}
		close(ch)
		sh.mu.Unlock()
	}
}

func (h *Hub) PublishSim(sessionID uuid.UUID, line string) {
	h.publish(sessionID, LogEvent{
		TS:   time.Now().UTC().Format(time.RFC3339),
		Line: line,
	}, true)
}

func (h *Hub) PublishDest(sessionID uuid.UUID, line, deliveryID, bodyPreview string) {
	ev := LogEvent{
		TS:          time.Now().UTC().Format(time.RFC3339),
		Line:        line,
		DeliveryID:  deliveryID,
		BodyPreview: bodyPreview,
	}
	h.publish(sessionID, ev, false)
}

func (h *Hub) publish(sessionID uuid.UUID, ev LogEvent, sim bool) {
	sh := h.get(sessionID)
	sh.mu.Lock()
	if !sim {
		sh.destLog = append(sh.destLog, ev)
		if len(sh.destLog) > maxDestLog {
			sh.destLog = sh.destLog[len(sh.destLog)-maxDestLog:]
		}
	}
	subs := sh.simSubs
	if !sim {
		subs = sh.destSubs
	}
	for ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
	sh.mu.Unlock()
}

func FormatSSE(eventType string, ev LogEvent) ([]byte, error) {
	data, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	return []byte("event: " + eventType + "\ndata: " + string(data) + "\n\n"), nil
}
