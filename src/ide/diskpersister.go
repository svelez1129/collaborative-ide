package ide

import (
	"log"
	"os"
	"sync"
)

// DiskPersister is a crash-safe replacement for the in-memory tester.Persister.
// It stores Raft state and the state-machine snapshot in two separate files on
// disk, one per Raft peer.  Writes are atomic: we write to a temp file and
// rename it over the target so a crash mid-write never leaves a corrupt file.
//
// File layout (dir is chosen by the caller, e.g. "/var/lib/gocollab/peer-0"):
//
//	<dir>/raftstate   – serialised Raft log, currentTerm, votedFor
//	<dir>/snapshot    – serialised CollabServer state-machine snapshot
type DiskPersister struct {
	mu            sync.Mutex
	dir           string
	raftstatePath string
	snapshotPath  string

	// in-memory cache so RaftStateSize() and SnapshotSize() are cheap
	raftstate []byte
	snapshot  []byte
}

// MakeDiskPersister creates (or reopens) a persister rooted at dir.
// If existing files are found they are read into the in-memory cache so Raft
// can restore its state immediately after a restart.
func MakeDiskPersister(dir string) *DiskPersister {
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("DiskPersister: mkdir %q: %v", dir, err)
	}
	dp := &DiskPersister{
		dir:           dir,
		raftstatePath: dir + "/raftstate",
		snapshotPath:  dir + "/snapshot",
	}
	// Load whatever survived the last run (errors just mean empty / first boot).
	dp.raftstate, _ = os.ReadFile(dp.raftstatePath)
	dp.snapshot, _ = os.ReadFile(dp.snapshotPath)
	return dp
}

// Save atomically persists both Raft state and the snapshot.
// Raft calls this whenever it updates either piece of state, passing nil for
// whichever hasn't changed.  We only skip the write if the caller explicitly
// passes nil (meaning "leave the file as-is").
func (dp *DiskPersister) Save(raftstate []byte, snapshot []byte) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if raftstate != nil {
		dp.raftstate = clone(raftstate)
		atomicWrite(dp.raftstatePath, dp.raftstate)
	}
	if snapshot != nil {
		dp.snapshot = clone(snapshot)
		atomicWrite(dp.snapshotPath, dp.snapshot)
	}
}

func (dp *DiskPersister) ReadRaftState() []byte {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return clone(dp.raftstate)
}

func (dp *DiskPersister) RaftStateSize() int {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return len(dp.raftstate)
}

func (dp *DiskPersister) ReadSnapshot() []byte {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return clone(dp.snapshot)
}

func (dp *DiskPersister) SnapshotSize() int {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return len(dp.snapshot)
}

// atomicWrite writes data to path using a write-then-rename pattern so a
// crash during the write can never leave a half-written file behind.
func atomicWrite(path string, data []byte) {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		log.Fatalf("DiskPersister: write %q: %v", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Fatalf("DiskPersister: rename %q -> %q: %v", tmp, path, err)
	}
}

// clone returns a copy of b so the caller cannot mutate our cached slice.
func clone(b []byte) []byte {
	if b == nil {
		return nil
	}
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
