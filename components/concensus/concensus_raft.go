package concensus

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/aarthikrao/timeMachine/components/concensus/fsm"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"go.uber.org/zap"
)

// TODO: Review all this
const (
	// The maxPool controls how many connections we will pool.
	maxPool = 3

	// The timeout is used to apply I/O deadlines. For InstallSnapshot, we multiply
	// the timeout by (SnapshotSize / TimeoutScale).
	// https://github.com/hashicorp/raft/blob/v1.1.2/net_transport.go#L177-L181
	tcpTimeout = 10 * time.Second

	// The `retain` parameter controls how many
	// snapshots are retained. Must be at least 1.
	raftSnapShotRetain = 2

	// raftLogCacheSize is the maximum number of logs to cache in-memory.
	// This is used to reduce disk I/O for the recently committed entries.
	raftLogCacheSize = 512
)

type raftConcensus struct {
	raft raft.Raft
}

func NewRaftConcensus(serverID string, port int, volumeDir string, log *zap.Logger) (*raftConcensus, error) {
	raftConf := raft.DefaultConfig()
	raftConf.LocalID = raft.ServerID(serverID)
	raftConf.SnapshotThreshold = 1024

	store, err := raftboltdb.NewBoltStore(filepath.Join(volumeDir, "raft.dataRepo"))
	if err != nil {
		return nil, err
	}

	// Wrap the store in a LogCache to improve performance.
	cacheStore, err := raft.NewLogCache(raftLogCacheSize, store)
	if err != nil {
		return nil, err
	}

	snapshotStore, err := raft.NewFileSnapshotStore(volumeDir, raftSnapShotRetain, os.Stdout)
	if err != nil {
		return nil, err
	}

	var raftBinAddr = fmt.Sprintf(":%d", port)
	tcpAddr, err := net.ResolveTCPAddr("tcp", raftBinAddr)
	if err != nil {
		return nil, err
	}

	transport, err := raft.NewTCPTransport(raftBinAddr, tcpAddr, maxPool, tcpTimeout, os.Stdout)
	if err != nil {
		return nil, err
	}

	fsmStore := fsm.NewConfigFSM(log)
	raftServer, err := raft.NewRaft(raftConf, fsmStore, cacheStore, store, snapshotStore, transport)
	if err != nil {
		return nil, err
	}

	return &raftConcensus{
		raft: *raftServer,
	}, nil
}

// Join is called to add a new node in the cluster.
// It returns an error if this node is not a leader
func (r *raftConcensus) Join(nodeID, raftAddress string) error {
	if r.raft.State() != raft.Leader {
		return ErrNotLeader
	}

	return r.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(raftAddress), 0, 0).Error()
}

// Remove is called to remove a particular node from the cluster.
// It returns an error if this node is not a leader
func (r *raftConcensus) Remove(nodeID string) error {
	if r.raft.State() != raft.Leader {
		return ErrNotLeader
	}

	return r.raft.RemoveServer(raft.ServerID(nodeID), 0, 0).Error()
}

// Stats returns the stats of raft on this node
func (r *raftConcensus) Stats() map[string]string {
	return r.raft.Stats()
}

// Returns true if the current node is leader
func (r *raftConcensus) IsLeader() bool {
	return r.raft.State() == raft.Leader
}
