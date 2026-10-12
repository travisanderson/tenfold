package pipeline

import (
	"sync"

	"github.com/google/uuid"

	"tenfold/backend/internal/api"
)

// Hub carries job progress from the running job to the SSE streams that
// watch it. It lives in memory, so it works for one server process. Run
// more than one and it must move to Postgres LISTEN/NOTIFY or similar.
type Hub struct {
	mu   sync.Mutex
	subs map[uuid.UUID]map[chan api.ProgressEvent]struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub {
	return &Hub{subs: map[uuid.UUID]map[chan api.ProgressEvent]struct{}{}}
}

// Subscribe returns a channel of one upload's progress and a function that
// ends the subscription.
//
// The channel holds one event. A slow reader misses the steps in between
// but always gets the newest, which is all a progress bar needs. It also
// means a stalled browser can never slow the job down.
func (h *Hub) Subscribe(uploadID uuid.UUID) (<-chan api.ProgressEvent, func()) {
	ch := make(chan api.ProgressEvent, 1)
	h.mu.Lock()
	if h.subs[uploadID] == nil {
		h.subs[uploadID] = map[chan api.ProgressEvent]struct{}{}
	}
	h.subs[uploadID][ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.subs[uploadID], ch)
		if len(h.subs[uploadID]) == 0 {
			delete(h.subs, uploadID)
		}
		h.mu.Unlock()
	}
}

// Publish sends an event to everyone watching the upload, replacing any
// event a subscriber has not read yet.
func (h *Hub) Publish(e api.ProgressEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[e.UploadId] {
		select {
		case <-ch: // drop the unread event
		default:
		}
		ch <- e
	}
}
