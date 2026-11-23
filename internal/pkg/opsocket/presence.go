package opsocket

import (
	"encoding/json"
	"sort"
	"strconv"
)

// PresenceEvent tells operators which contestant stations are connected. It is
// sent to role:admin whenever any client connects or disconnects.
const PresenceEvent = "Contestant.Presence"

// ContestantPresence lists the contestant slots with at least one live station.
// A slot missing from the list is offline.
type ContestantPresence struct {
	Slots []int `json:"slots"`
}

// onlineSlots returns the sorted, de-duplicated contestant slots among clients.
func onlineSlots(clients []*Client) []int {
	seen := make(map[int]struct{})
	for _, client := range clients {
		if client == nil {
			continue
		}
		slot, err := strconv.Atoi(client.Principal.Claims["slot"])
		if err != nil || slot <= 0 {
			continue
		}
		seen[slot] = struct{}{}
	}
	slots := make([]int, 0, len(seen))
	for slot := range seen {
		slots = append(slots, slot)
	}
	sort.Ints(slots)
	return slots
}

// broadcastPresence sends the current contestant presence to every operator.
// Call it after a client has been added to or removed from s.clients.
func (s *baseSocket) broadcastPresence() {
	s.clientsMutex.RLock()
	clients := make([]*Client, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	s.clientsMutex.RUnlock()
	payload, err := json.Marshal(ContestantPresence{Slots: onlineSlots(clients)})
	if err != nil {
		return
	}
	s.EmitEventToRoom("role:admin", PresenceEvent, payload)
}
