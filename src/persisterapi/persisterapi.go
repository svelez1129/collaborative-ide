package persisterapi

// Persister is the interface that Raft uses to durably save its state.
// Both tester.Persister (in-memory, for lab tests) and ide.DiskPersister
// (file-backed, for production) satisfy this interface.
type Persister interface {
	// Save atomically writes both pieces of state. Pass nil to leave
	// the corresponding piece unchanged.
	Save(raftstate []byte, snapshot []byte)

	ReadRaftState() []byte
	RaftStateSize() int

	ReadSnapshot() []byte
	SnapshotSize() int
}
