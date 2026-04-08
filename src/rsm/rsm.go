package rsm

import (
	"sync"
	"time"

	"github.com/svelez1129/collaborative-ide/src/labrpc"
	"github.com/svelez1129/collaborative-ide/src/persisterapi"
	raft "github.com/svelez1129/collaborative-ide/src/raft"
	"github.com/svelez1129/collaborative-ide/src/raftapi"
	"github.com/svelez1129/collaborative-ide/src/rpc"
)

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	Req any
	Id  int64
}

// A server (i.e., ../server.go) that wants to replicate itself calls
// MakeRSM and must implement the StateMachine interface.  This
// interface allows the rsm package to interact with the server for
// server-specific operations: the server must implement DoOp to
// execute an operation (e.g., a Get or Put request), and
// Snapshot/Restore to snapshot and restore the server's state.
type StateMachine interface {
	DoOp(any) any
	Snapshot() []byte
	Restore([]byte)
}

type RSM struct {
	mu           sync.Mutex
	me           int
	rf           raftapi.Raft
	applyCh      chan raftapi.ApplyMsg
	maxraftstate int // snapshot if log grows this big
	sm           StateMachine
	// Your definitions here.
	nextId    int64
	channels  map[int]chan OpResult
	chanIds   map[int]int64
	chanTerms map[int]int
}

// struct for what channel that submit waits for has
type OpResult struct {
	Err   rpc.Err
	Value any
}

// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
//
// me is the index of the current server in servers[].
//
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// The RSM should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
//
// MakeRSM() must return quickly, so it should start goroutines for
// any long-running work.
func MakeRSM(servers []*labrpc.ClientEnd, me int, persister persisterapi.Persister, maxraftstate int, sm StateMachine) *RSM {
	rsm := &RSM{
		me:           me,
		maxraftstate: maxraftstate,
		applyCh:      make(chan raftapi.ApplyMsg),
		sm:           sm,
		//map of channels
		channels: make(map[int]chan OpResult),
		//map of channel ids, given index return channel id
		chanIds: make(map[int]int64),
		//map of channel terms, if channel term changed we changed leaders
		chanTerms: make(map[int]int),
	}
	rsm.rf = raft.Make(servers, me, persister, rsm.applyCh)
	//see if we have a snapshot
	snapshot := persister.ReadSnapshot()
	if len(snapshot) > 0 {
		sm.Restore(snapshot)
	}
	go rsm.reader()
	go rsm.leaderCheck()
	return rsm
}

func (rsm *RSM) Raft() raftapi.Raft {
	return rsm.rf
}

// Submit a command to Raft, and wait for it to be committed.  It
// should return ErrWrongLeader if client should find new leader and
// try again.
func (rsm *RSM) Submit(req any) (rpc.Err, any) {

	// Submit creates an Op structure to run a command through Raft;
	// for example: op := Op{Me: rsm.me, Id: id, Req: req}, where req
	// is the argument to Submit and id is a unique id for the op.
	rsm.mu.Lock()
	rsm.nextId++
	id := int64(rsm.me)<<32 | rsm.nextId
	rsm.mu.Unlock()
	op := Op{Req: req, Id: id}
	index, term, isLeader := rsm.rf.Start(op)
	//if we aren't the leader return an error
	if !isLeader {
		return rpc.ErrWrongLeader, nil
	}
	rsm.mu.Lock()
	//if we are leader create channel at channels[index], and wait until getting a response
	currChannel := make(chan OpResult, 1)
	rsm.channels[index] = currChannel
	rsm.chanIds[index] = id
	rsm.chanTerms[index] = term
	rsm.mu.Unlock()
	result := <-currChannel
	rsm.mu.Lock()
	//since we already used the channel, delete it
	delete(rsm.channels, index)
	delete(rsm.chanIds, index)
	delete(rsm.chanTerms, index)
	rsm.mu.Unlock()
	return result.Err, result.Value
}

// reader go routine, for readers to apply commited messages and execute them
func (rsm *RSM) reader() {
	for message := range rsm.applyCh {
		//if the message is a snapshot we restore
		if message.SnapshotValid {
			rsm.sm.Restore(message.Snapshot)
			rsm.mu.Lock()
			//since we restored snapshot, wakeup all those waiting
			for idx := range rsm.channels {
				rsm.channels[idx] <- OpResult{Err: rpc.ErrWrongLeader, Value: nil}
				delete(rsm.channels, idx)
				delete(rsm.chanIds, idx)
				delete(rsm.chanTerms, idx)
			}
			rsm.mu.Unlock()
			continue
		}

		//if the command is not valid just continue
		if !message.CommandValid {
			continue
		}
		//get operation back
		op,ok := message.Command.(Op)
		if !ok{
			continue
		}
		//since sm(state machine) never changes once set, don't need to lock it
		result := rsm.sm.DoOp(op.Req)
		//check if we need to take a snapshot
		if rsm.maxraftstate != -1 && rsm.rf.PersistBytes() >= rsm.maxraftstate {
			snapshot := rsm.sm.Snapshot()
			rsm.rf.Snapshot(message.CommandIndex, snapshot)
		}
		rsm.mu.Lock()
		//get the index of the channel, if no one is waiting for the channel continue
		idx := message.CommandIndex
		if rsm.channels[idx] == nil {
			rsm.mu.Unlock()
			continue
		}
		//if the operation ids don't match we have a new leader so return an error
		if op.Id != rsm.chanIds[idx] {
			rsm.channels[idx] <- OpResult{Err: rpc.ErrWrongLeader, Value: nil}
			rsm.mu.Unlock()
			continue
		}
		//if it does match we don't return an error
		rsm.channels[idx] <- OpResult{Err: rpc.OK, Value: result}
		rsm.mu.Unlock()
	}
	//if applyCh closes wake up all people waiting
	rsm.mu.Lock()
	for idx := range rsm.channels {
		rsm.channels[idx] <- OpResult{Err: rpc.ErrWrongLeader, Value: nil}
	}
	rsm.mu.Unlock()
}

// check if the term or isLeader state for rf changed, wake everyone up since leader changed
func (rsm *RSM) leaderCheck() {
	for {
		time.Sleep(10 * time.Millisecond)
		currTerm, _ := rsm.rf.GetState()
		rsm.mu.Lock()
		for idx, chanTerm := range rsm.chanTerms {
			if chanTerm < currTerm {
				rsm.channels[idx] <- OpResult{Err: rpc.ErrWrongLeader, Value: nil}
			}
		}
		rsm.mu.Unlock()
	}
}
