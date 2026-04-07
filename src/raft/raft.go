package raft

// The file ../raftapi/raftapi.go defines the interface that raft must
// expose to servers (or the tester), but see comments below for each
// of these functions for more details.
//
// In addition,  Make() creates a new raft peer that implements the
// raft interface.

import (
	"bytes"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/svelez1129/collaborative-ide/src/labgob"
	"github.com/svelez1129/collaborative-ide/src/labrpc"
	"github.com/svelez1129/collaborative-ide/src/raftapi"
	tester "github.com/svelez1129/collaborative-ide/src/tester1"
)

// define role types
type Role int

const (
	// 0 is follower, 1 is candidate, 2 is leader
	Follower Role = iota
	Candidate
	Leader
)

type LogEntry struct {
	//term of the current log entry
	Term int
	//command of the log entry
	Command interface{}
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *tester.Persister   // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]

	// Your data here (3A, 3B, 3C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	currentTerm   int
	votedFor      int
	currentRole   Role
	lastHeartBeat time.Time
	//randomized election timeout
	electionTimeout time.Duration
	log             []LogEntry
	//index of highest log entry known to be commited
	commitIndex int
	//index of highest log entry applied to state machine
	lastApplied int
	//for each server, index of the next log entry to send to that server
	nextIndex []int
	//for each server, index of highest log entry known to be replicated on server
	matchIndex []int
	//channel thats sends commited log entries to server
	applyChan chan raftapi.ApplyMsg
	// last included index and term to implement snapshot
	lastIncludedIndex int
	lastIncludedTerm  int
	snapshot          []byte
	//used for lab 4, indicates if rf is killed or not
	killed int32
	//used in lab4, when start() is called, notifies heartbeat
	newLogEntry chan struct{}
}

// kill raft, killing its applyChan
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.killed, 1)
	close(rf.applyChan)
}

// check if raft is dead or not
func (rf *Raft) isDead() bool {
	isDead := atomic.LoadInt32(&rf.killed) == 1
	return isDead
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (3A).
	rf.mu.Lock()
	isleader = rf.currentRole == Leader
	term = rf.currentTerm
	rf.mu.Unlock()
	return term, isleader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persist() {
	// Your code here (3C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// raftstate := w.Bytes()
	// rf.persister.Save(raftstate, nil)
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	//persistent state on all servers is currentTerm, votedFor, and log[]
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)
	e.Encode(rf.lastIncludedIndex)
	e.Encode(rf.lastIncludedTerm)
	raftstate := w.Bytes()
	//save snapshot
	snapshot := rf.snapshot
	rf.persister.Save(raftstate, snapshot)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (3C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	// read the persist state of currentTerm, votedFor, and logEntry
	var currentTerm int
	var votedFor int
	var log []LogEntry
	var lastIncludedIndex int
	var lastIncludedTerm int
	//encodes happen in same order as decodes, which is why we have it in this order
	if d.Decode(&currentTerm) != nil || d.Decode(&votedFor) != nil ||
		d.Decode(&log) != nil || d.Decode(&lastIncludedIndex) != nil || d.Decode(&lastIncludedTerm) != nil {
		return
	} else {
		rf.currentTerm = currentTerm
		rf.votedFor = votedFor
		rf.log = log
		rf.lastIncludedIndex = lastIncludedIndex
		rf.lastIncludedTerm = lastIncludedTerm
	}
}

// how many bytes in Raft's persisted log?
func (rf *Raft) PersistBytes() int {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.persister.RaftStateSize()
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (3D).
	rf.mu.Lock()
	//already snapshotted this, so return
	if index <= rf.lastIncludedIndex || rf.logIndex(index) >= len(rf.log) {
		rf.mu.Unlock()
		return
	}
	rf.lastIncludedTerm = rf.log[rf.logIndex(index)].Term
	newLog := []LogEntry{{Term: rf.lastIncludedTerm, Command: nil}}
	newLog = append(newLog, rf.log[rf.logIndex(index)+1:]...)
	rf.lastIncludedIndex = index
	rf.log = newLog
	rf.snapshot = snapshot
	rf.persist()
	rf.mu.Unlock()
}

// rpc for installing snapshot
type InstallSnapshotArgs struct {
	// leader's term
	Term int
	// so follower can redirect clients
	LeaderId int
	//the snapshot replaces all entries up through and including this index
	LastIncludedIndex int
	// term of lastIncludedIndex
	LastIncludedTerm int
	//raw bytes of the snapshot chunk, starting at offset
	Data []byte
}

// rpc for installing snapshot
type InstallSnapshotReply struct {
	// current term, for leader to update itself
	Term int
}

// send install snapshot to followers
// makes the other followers install current snapshot of the log
func (rf *Raft) sendInstallSnapshot(server int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) bool {
	ok := rf.peers[server].Call("Raft.InstallSnapshot", args, reply)
	return ok
}

// installSnapShot RPC handler
func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	rf.mu.Lock()
	// if leaders term is less than yours, return your term and reject this
	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		rf.mu.Unlock()
		return
	}
	//if leader term is valid become a follower and reset election timer
	rf.resetElectionTimer()
	rf.currentRole = Follower
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
	}
	reply.Term = args.Term
	//check if the snapshot is stale
	if args.LastIncludedIndex <= rf.lastIncludedIndex {
		rf.mu.Unlock()
		return
	}
	// if its not stale, replace our log
	if args.LastIncludedIndex <= rf.lastLogIndex() && rf.log[rf.logIndex(args.LastIncludedIndex)].Term == args.LastIncludedTerm {
		// keep the entries after the snapshot
		newLog := []LogEntry{{Term: args.LastIncludedTerm, Command: nil}}
		newLog = append(newLog, rf.log[rf.logIndex(args.LastIncludedIndex)+1:]...)
		rf.log = newLog
	} else {
		// if the log doesn't extend past the snapshot, make a new log
		rf.log = []LogEntry{{Term: args.LastIncludedTerm, Command: nil}}
	}
	rf.lastIncludedIndex = args.LastIncludedIndex
	rf.lastIncludedTerm = args.LastIncludedTerm
	rf.snapshot = args.Data
	if args.LastIncludedIndex > rf.commitIndex {
		rf.commitIndex = args.LastIncludedIndex
	}
	rf.persist()
	rf.mu.Unlock()
}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (3A, 3B).
	// candidate's term
	Term int
	// candidate requesting vote
	CandidateId int
	// index of candidate's last log entry
	LastLogIndex int
	//term of the candidate's last log entry
	LastLogTerm int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (3A).
	// currentTerm, for candidate to update itself
	Term int
	// true means candidate recieved vote
	VoteGranted bool
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (3A, 3B).
	// if our current term is greater than the candidate's, don't grant a vote
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// if our term is greater than the person wanting our vote
	if args.Term < rf.currentTerm {
		reply.VoteGranted = false
		reply.Term = rf.currentTerm
		return
		//if our term is less than the person wanting our vote
	} else if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.currentRole = Follower
		rf.votedFor = -1
		rf.persist()
	}
	//if both terms match
	reply.Term = rf.currentTerm
	// if we have not voted for anyone yet or voted for this person
	if rf.votedFor == -1 || rf.votedFor == args.CandidateId {
		// now check if our log is up to date with the candidates
		currLastLogIndex := rf.lastLogIndex()
		currLastLogTerm := rf.log[rf.logIndex(currLastLogIndex)].Term
		// if leader is up to date, grant the vote
		if currLastLogTerm < args.LastLogTerm || (currLastLogTerm == args.LastLogTerm && currLastLogIndex <= args.LastLogIndex) {
			reply.VoteGranted = true
			//reset election timer and randomize election timeout
			rf.resetElectionTimer()
			rf.votedFor = args.CandidateId
			rf.persist()
			return
		}
	}
	//if we already vote for someone do not grant the vote
	reply.VoteGranted = false
}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

// stuff for managing heartbeats
// append entries rpc for arguments
type AppendEntriesArgs struct {
	// current leaders term
	Term int
	//current leader id
	LeaderId int
	//index of log entry immediately preceding new ones
	PrevLogIndex int
	//term of prevLogIndex entry
	PrevLogTerm int
	//log entries to store
	Entries []LogEntry
	//leader's commit index
	LeaderCommit int
}

// stuff for managing heartbeats
// append entries rpc for replies
type AppendEntriesReply struct {
	// current term, for leader to update itself
	Term int
	// true if follower contained entry matching prevLogIndex and prevLogTerm
	Success bool
	//optimization for returning
	//term in the conflicting entry (if any)
	XTerm int
	//index of first entry with that term (if any)
	XIndex int
	//log length
	XLen int
}

// example Heartbeat RPC handler.
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	// Your code here (3A, 3B).
	// if our current term is greater than the candidate's, don't grant a vote
	rf.mu.Lock()
	defer rf.mu.Unlock()
	//if our term is greater than the leader's, update the leader, stale leader so dont reset election timer
	if rf.currentTerm > args.Term {
		reply.Term = rf.currentTerm
		reply.Success = false
		return
		//if our term is less than leaders, update our term and voted For to -1
	} else if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.votedFor = -1
		rf.persist()
	}
	rf.currentRole = Follower
	//reset election timer because we just got a heartbeat
	rf.resetElectionTimer()
	reply.Term = rf.currentTerm
	//do log consistency check after resetting election timer
	//check if prevLogIndex matches is greater than ours
	if args.PrevLogIndex > rf.lastLogIndex() {
		//find conflicting entry to return it
		reply.XLen = rf.lastLogIndex() + 1
		//no conflicting entry, just that prevlogindex is greater than our current log
		reply.XIndex = -1
		reply.XTerm = -1
		reply.Success = false
		return
	}
	if args.PrevLogIndex < rf.lastIncludedIndex {
		reply.Success = false
		reply.XTerm = -1
		reply.XIndex = -1
		reply.XLen = rf.lastIncludedIndex + 1
		return
	}
	if rf.log[rf.logIndex(args.PrevLogIndex)].Term != args.PrevLogTerm {
		//find conflicting entry to return it
		reply.XLen = rf.lastLogIndex() + 1
		//find the conflictingTerm
		conflictingTerm := rf.log[rf.logIndex(args.PrevLogIndex)].Term
		reply.XTerm = conflictingTerm
		//find first index of the conflicting term
		for i := 0; i < len(rf.log); i++ {
			if rf.log[i].Term == conflictingTerm {
				//get the real last index
				reply.XIndex = i + rf.lastIncludedIndex
				break
			}
		}
		reply.Success = false
		return
	}
	//if the log index to append to is valid
	rf.log = append(rf.log[:rf.logIndex(args.PrevLogIndex)+1], args.Entries...)
	rf.persist()
	if args.LeaderCommit > rf.commitIndex {
		//set commit index to min(leaderCommit, index of last new entry)
		if args.LeaderCommit < rf.lastLogIndex() {
			rf.commitIndex = args.LeaderCommit
		} else {
			rf.commitIndex = rf.lastLogIndex()
		}
	}
	reply.Success = true
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) heartbeat() {
	for !rf.isDead() {
		//send heartbeats
		rf.mu.Lock()
		if rf.currentRole != Leader {
			rf.mu.Unlock()
			return
		}
		currentTerm := rf.currentTerm
		me := rf.me
		//append entry argument for each follower
		appendEntriesArgsList := make([]*AppendEntriesArgs, len(rf.peers))
		// snapshot argument for each follower
		installSnapshotArgsList := make([]*InstallSnapshotArgs, len(rf.peers))
		//majority of peers
		majority := len(rf.peers)/2 + 1
		for i := range rf.peers {
			//skip yourself
			if i == me {
				continue
			}
			//if the next index is less than or equal to the last included index, we need to send a snapshot
			if rf.nextIndex[i] <= rf.lastIncludedIndex {
				data := rf.snapshot
				args := &InstallSnapshotArgs{
					Term:              currentTerm,
					LeaderId:          me,
					LastIncludedIndex: rf.lastIncludedIndex,
					LastIncludedTerm:  rf.lastIncludedTerm,
					Data:              data,
				}
				installSnapshotArgsList[i] = args
			} else {
				//if not we just append entry
				//last entry follower should already have
				prevLogIndex := rf.nextIndex[i] - 1
				//term of last entry follower should have
				prevLogTerm := rf.log[rf.logIndex(prevLogIndex)].Term
				//new entries to send to followers
				entries := make([]LogEntry, len(rf.log[rf.logIndex(rf.nextIndex[i]):]))
				copy(entries, rf.log[rf.logIndex(rf.nextIndex[i]):])
				args := &AppendEntriesArgs{
					Term:         currentTerm,
					LeaderId:     me,
					PrevLogIndex: prevLogIndex,
					PrevLogTerm:  prevLogTerm,
					Entries:      entries,
					LeaderCommit: rf.commitIndex,
				}
				appendEntriesArgsList[i] = args
			}
		}
		rf.mu.Unlock()
		for i := range rf.peers {
			//leader skips over themselves
			if i == me {
				continue
			}
			go func(targetNode int) {
				reply := &AppendEntriesReply{}
				if installSnapshotArgsList[targetNode] == nil {
					args := appendEntriesArgsList[targetNode]
					ok := rf.sendAppendEntries(targetNode, args, reply)
					if ok {
						rf.mu.Lock()
						defer rf.mu.Unlock()
						// if the replies term is greater than our term, we can no longer be the leader
						if reply.Term > currentTerm {
							rf.currentTerm = reply.Term
							rf.currentRole = Follower
							rf.votedFor = -1
							rf.persist()
							return
						}
						//if we have a different term or aren't the leader, return
						if rf.currentTerm != currentTerm || rf.currentRole != Leader {
							return
						}
						// if reply was not successful, retry with lower index
						if reply.Success == false {
							/*
								Case 1: leader doesn't have XTerm:
								nextIndex = XIndex
								Case 2: leader has XTerm:
								nextIndex = (index of leader's last entry for XTerm) + 1
								Case 3: follower's log is too short:
								nextIndex = XLen
							*/
							//Case 3: follower's log is too short, meaning there is no conflicting term
							if reply.XTerm == -1 {
								rf.nextIndex[targetNode] = reply.XLen
							} else {
								//Case 1 and 2
								lastXTermIndex := -1
								for i := rf.lastLogIndex(); i >= rf.lastIncludedIndex; i-- {
									if rf.log[rf.logIndex(i)].Term == reply.XTerm {
										lastXTermIndex = i
										break
									}
								}
								//Case 1: leader doesn't have Xterm:
								if lastXTermIndex == -1 {
									rf.nextIndex[targetNode] = reply.XIndex
									// Case 2 leader has XTerm
								} else {
									rf.nextIndex[targetNode] = lastXTermIndex + 1
								}
							}
						}
						//if it was a success
						if reply.Success == true {
							//next index is the prevlogindex+ the entries we added+1
							rf.nextIndex[targetNode] = args.PrevLogIndex + len(args.Entries) + 1
							//all entries except whatever comes next have been replicated
							rf.matchIndex[targetNode] = rf.nextIndex[targetNode] - 1
							//for all indexes between current log entry and commitIndex
							for i := rf.lastLogIndex(); i > rf.commitIndex; i-- {
								//leader can only commit entries from its own term
								if rf.log[rf.logIndex(i)].Term != currentTerm {
									continue
								}
								//count the leader itself
								count := 1
								// for each peer, check if its matchIndex is greater than or equal to i
								for j := range rf.peers {
									if j == me {
										continue
									}
									if rf.matchIndex[j] >= i {
										count++
									}
								}
								//if a majority of peers have replicated at i, set new commit index
								if count >= majority {
									rf.commitIndex = i
									break
								}
							}
						}
					}
				} else {
					//need to use snapshot
					args := installSnapshotArgsList[targetNode]
					reply := &InstallSnapshotReply{}
					ok := rf.sendInstallSnapshot(targetNode, args, reply)
					if ok {
						rf.mu.Lock()
						defer rf.mu.Unlock()
						if reply.Term > currentTerm {
							//if the returned term is greater than ours, we can no longer be leader
							rf.currentTerm = reply.Term
							rf.currentRole = Follower
							rf.votedFor = -1
							rf.persist()
							return
						}
						// if we arent in the current term or arent leader return
						if rf.currentTerm != currentTerm || rf.currentRole != Leader {
							return
						}
						rf.nextIndex[targetNode] = args.LastIncludedIndex + 1
						rf.matchIndex[targetNode] = args.LastIncludedIndex
					}
				}
			}(i)
		}
		select {
		//if we got a newLogEntry replicate it now
		case <-rf.newLogEntry:
		//if not wait 100ms like standard heartbeat
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (3B).
	//since we arent a leader we cannot append to our own log
	rf.mu.Lock()
	if rf.currentRole != Leader {
		isLeader = false
	} else {
		index = rf.lastLogIndex() + 1
		rf.log = append(rf.log, LogEntry{Term: rf.currentTerm, Command: command})
		select {
		//notifies heartbeat that we got a new log entry so it wakes up
		case rf.newLogEntry <- struct{}{}:
		default:
		}
		rf.persist()
	}
	term = rf.currentTerm
	rf.mu.Unlock()
	return index, term, isLeader
}

// helper function to reset election timet,
// resets timer between 300-500ms
func (rf *Raft) resetElectionTimer() {
	rf.lastHeartBeat = time.Now()
	newMs := 300 + (rand.Int63() % 200)
	newTimeout := time.Duration(newMs) * time.Millisecond
	rf.electionTimeout = newTimeout
}

// helper function get index of log given position to account for lastIncludedIndex
func (rf *Raft) logIndex(currIndex int) int {
	return currIndex - rf.lastIncludedIndex
}

// helper function get real index of the last entry of our log
func (rf *Raft) lastLogIndex() int {
	return len(rf.log) + rf.lastIncludedIndex - 1
}

func (rf *Raft) ticker() {
	for !rf.isDead() {
		// Check if a leader election should be started.
		//if a node isn't a leader and has timed out, make it start an election
		rf.mu.Lock()
		if rf.currentRole != Leader && time.Since(rf.lastHeartBeat) > rf.electionTimeout {
			rf.currentRole = Candidate
			rf.currentTerm += 1
			rf.votedFor = rf.me
			rf.persist()
			//reset election timer and randomize election timeout
			rf.resetElectionTimer()
			// hold current term and me as local variables since we unlock the lock
			currentTerm := rf.currentTerm
			me := rf.me
			lastLogIndex := rf.lastLogIndex()
			lastLogTerm := rf.log[rf.logIndex(lastLogIndex)].Term
			rf.mu.Unlock()
			//request locks
			// fill in args
			args := &RequestVoteArgs{
				Term:         currentTerm,
				CandidateId:  me,
				LastLogIndex: lastLogIndex,
				LastLogTerm:  lastLogTerm,
			}
			voteCount := 1
			votesNeeded := (len(rf.peers) / 2) + 1
			for i := range rf.peers {
				//already voted for yourself so skip
				if i == me {
					continue
				}
				//start go routine so voting can happen in parallel
				go func(targetNode int) {
					reply := &RequestVoteReply{}
					ok := rf.sendRequestVote(targetNode, args, reply)
					if ok {
						rf.mu.Lock()
						defer rf.mu.Unlock()
						//if there is already a greater term we are a follower
						if reply.Term > currentTerm {
							rf.currentRole = Follower
							rf.currentTerm = reply.Term
							rf.votedFor = -1
							rf.persist()
							return
						}
						//if we are not in the same term or role we were in return
						if rf.currentTerm != currentTerm || rf.currentRole != Candidate {
							return
						}
						if reply.VoteGranted == true {
							voteCount += 1
						}
						if voteCount >= votesNeeded && rf.currentRole != Leader {
							rf.currentRole = Leader
							// change volatile state on leaders
							rf.nextIndex = make([]int, len(rf.peers))
							rf.matchIndex = make([]int, len(rf.peers))
							//leaders log index
							leaderLogIndex := rf.lastLogIndex()
							for i := range rf.nextIndex {
								rf.nextIndex[i] = leaderLogIndex + 1
								rf.matchIndex[i] = 0
							}
							go rf.heartbeat()
						}
						return
					}
				}(i)
			}
		} else {
			rf.mu.Unlock()
		}

		// pause for a random amount of time between 50 and 350
		// milliseconds.
		ms := 50 + (rand.Int63() % 300)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}

// go routine that happens constatly, applies commited entries to current node
func (rf *Raft) applier() {
	for !rf.isDead() {
		// Check if a leader election should be started.
		//if a node isn't a leader and has timed out, make it start an election
		rf.mu.Lock()
		//snapshot was installed that we have not applied yet, send it
		if rf.lastApplied < rf.lastIncludedIndex {
			rf.lastApplied = rf.lastIncludedIndex
			rf.mu.Unlock()
			if !rf.isDead() {
				rf.applyChan <- raftapi.ApplyMsg{
					SnapshotValid: true,
					Snapshot:      rf.snapshot,
					SnapshotIndex: rf.lastIncludedIndex,
					SnapshotTerm:  rf.lastIncludedTerm,
				}
			}
			continue
		}
		if rf.commitIndex > rf.lastApplied {
			if rf.lastApplied < rf.lastIncludedIndex {
				rf.mu.Unlock()
				continue
			}
			applyEntries := make([]LogEntry, rf.logIndex(rf.commitIndex)-rf.logIndex(rf.lastApplied))
			copy(applyEntries, rf.log[rf.logIndex(rf.lastApplied+1):rf.logIndex(rf.commitIndex)+1])
			startIndex := rf.lastApplied + 1
			rf.lastApplied = rf.commitIndex
			rf.mu.Unlock()
			for i, entry := range applyEntries {
				//if rf died dont send channel
				if rf.isDead() {
					return
				}
				//apply command to node
				rf.applyChan <- raftapi.ApplyMsg{
					CommandValid: true,
					Command:      entry.Command,
					CommandIndex: startIndex + i,
				}
			}
		} else {
			rf.mu.Unlock()
		}
		// sleep for 10 milliseconds if nothing to do
		time.Sleep(time.Duration(10) * time.Millisecond)
	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *tester.Persister, applyCh chan raftapi.ApplyMsg) raftapi.Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (3A, 3B, 3C).
	rf.currentTerm = 0
	// -1 represents that current node has voted for no one
	rf.votedFor = -1
	rf.currentRole = Follower
	rf.lastHeartBeat = time.Now()
	// timeout between 300ms and 500ms
	ms := 300 + (rand.Int63() % 200)
	timeout := time.Duration(ms) * time.Millisecond
	rf.electionTimeout = timeout
	//initialize log with dummy entry
	rf.log = []LogEntry{{Term: 0, Command: nil}}
	//volatile state on all servers
	rf.commitIndex = 0
	rf.lastApplied = 0
	//set applyChan
	rf.applyChan = applyCh
	//set last included term and index
	rf.lastIncludedIndex = 0
	rf.lastIncludedTerm = 0
	rf.snapshot = persister.ReadSnapshot()
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	rf.lastApplied = rf.lastIncludedIndex
	rf.commitIndex = rf.lastIncludedIndex
	rf.newLogEntry = make(chan struct{}, 1)
	// start ticker goroutine to start elections
	go rf.ticker()
	go rf.applier()

	return rf
}
