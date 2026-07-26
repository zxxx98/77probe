package live

import (
	"sync"
	"time"

	"probe.local/monitor/internal/protocol"
	"probe.local/monitor/internal/servers"
)

type Snapshot struct {
	ServerID       int64                `json:"serverId"`
	ServerName     string               `json:"serverName"`
	Online         bool                 `json:"online"`
	LastReceivedAt time.Time            `json:"lastReceivedAt"`
	SourceIP       string               `json:"sourceIp"`
	Report         protocol.AgentReport `json:"report"`
}

type Store struct {
	mu        sync.RWMutex
	snapshots map[int64]Snapshot
}

func NewStore() *Store {
	return &Store{snapshots: make(map[int64]Snapshot)}
}

func (s *Store) Upsert(server servers.Server, report protocol.AgentReport, receivedAt time.Time) Snapshot {
	return s.UpsertFrom(server, report, receivedAt, "")
}

func (s *Store) UpsertFrom(server servers.Server, report protocol.AgentReport, receivedAt time.Time, sourceIP string) Snapshot {
	snapshot := Snapshot{
		ServerID:       server.ID,
		ServerName:     server.Name,
		Online:         true,
		LastReceivedAt: receivedAt,
		SourceIP:       sourceIP,
		Report:         report,
	}
	s.mu.Lock()
	s.snapshots[server.ID] = snapshot
	s.mu.Unlock()
	return snapshot
}

func (s *Store) Get(serverID int64) (Snapshot, bool) {
	s.mu.RLock()
	snapshot, ok := s.snapshots[serverID]
	s.mu.RUnlock()
	return snapshot, ok
}

func (s *Store) MarkOffline(cutoff time.Time) []Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := make([]Snapshot, 0)
	for serverID, snapshot := range s.snapshots {
		if snapshot.Online && snapshot.LastReceivedAt.Before(cutoff) {
			snapshot.Online = false
			s.snapshots[serverID] = snapshot
			changed = append(changed, snapshot)
		}
	}
	return changed
}
