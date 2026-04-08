package ide

// peernet.go — wires Raft peers together over real Unix-domain sockets.
//
// The lab's labrpc.ClientEnd has a SetCall hook:
//
//	type Fcall func(endname string, svcMeth string, args []byte) ([]byte, bool)
//
// When set, ClientEnd.Call() invokes the Fcall instead of going through the
// simulated in-memory network. We use this to route every Raft RPC
// (RequestVote, AppendEntries, InstallSnapshot) over a real sockrpc connection.
//
// Layout per peer (peer index i, total n peers):
//
//	One RPCSrv   listening at socket  /tmp/6.5840-raft-<i>
//	n RPCClnts   connecting to        /tmp/6.5840-raft-<j>  for j != i
//
// The Raft struct is registered with the RPCSrv so the labrpc dispatch
// layer can reflect-call RequestVote / AppendEntries / InstallSnapshot
// exactly as it does in the lab tester.

import (
	"fmt"
	"log"
	"sync"

	"github.com/svelez1129/collaborative-ide/src/labrpc"
	"github.com/svelez1129/collaborative-ide/src/raftapi"
	"github.com/svelez1129/collaborative-ide/src/sockrpc"
)

const sockPrefix = "raft-"

// peerName returns the socket name for peer i.
func peerName(i int) string {
	return fmt.Sprintf("%s%d", sockPrefix, i)
}

// PeerNetwork holds the RPC server and outbound clients for one Raft peer.
// Call Close() when the peer shuts down.
type PeerNetwork struct {
	me     int
	n      int
	rpcs   *sockrpc.RPCSrv
	lazies []*lazyClient     // lazies[j] is the lazy connection to peer j (nil for j==me)
	ends   []*labrpc.ClientEnd
}

// lazyClient dials a peer on the first RPC and reconnects after failures.
// This lets all peers start in any order without timing dependencies.
type lazyClient struct {
	mu   sync.Mutex
	from string // our socket name
	to   string // target socket name
	clnt *sockrpc.RPCClnt
}

func (lc *lazyClient) RPC(svcMeth string, args []byte) ([]byte, bool) {
	lc.mu.Lock()
	if lc.clnt == nil {
		// Non-fatal: if the peer isn't up yet, return false so Raft retries later.
		lc.clnt = sockrpc.TryNewRPCClnt(lc.from, lc.to)
		if lc.clnt == nil {
			lc.mu.Unlock()
			return nil, false
		}
	}
	clnt := lc.clnt
	lc.mu.Unlock()

	rep, ok := clnt.RPC(svcMeth, args)
	if !ok {
		// Connection may be dead — drop it so the next call reconnects.
		lc.mu.Lock()
		if lc.clnt == clnt {
			lc.clnt = nil
		}
		lc.mu.Unlock()
	}
	return rep, ok
}

func (lc *lazyClient) Close() {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.clnt != nil {
		lc.clnt.Close()
		lc.clnt = nil
	}
}

// NewPeerNetwork creates the RPCSrv (starts listening immediately) and
// sets up lazy outbound connections to every other peer. Peers can start
// in any order — the connection is established on the first RPC call.
func NewPeerNetwork(me, n int) *PeerNetwork {
	pn := &PeerNetwork{
		me:     me,
		n:      n,
		lazies: make([]*lazyClient, n),
		ends:   make([]*labrpc.ClientEnd, n),
	}

	// Start listening for inbound Raft RPCs from other peers.
	pn.rpcs = sockrpc.NewRPCSrv(peerName(me))

	// Build a labrpc.ClientEnd for each peer with a custom Fcall that
	// routes over sockrpc instead of the simulated network.
	fakeNet := labrpc.MakeNetwork()
	lazies := make([]*lazyClient, n)
	for j := 0; j < n; j++ {
		end := fakeNet.MakeEnd(fmt.Sprintf("peer-%d->%d", me, j))
		if j != me {
			j := j // capture for closure
			lc := &lazyClient{from: peerName(me), to: peerName(j)}
			lazies[j] = lc

			// Override Call() to go through the real socket instead of
			// labrpc's simulated in-memory network.
			end.SetCall(func(_ string, svcMeth string, args []byte) ([]byte, bool) {
				return lc.RPC(svcMeth, args)
			})
		}
		pn.ends[j] = end
	}
	pn.lazies = lazies

	return pn
}

// RegisterRaft registers the Raft struct with the RPCSrv so incoming RPCs
// (RequestVote, AppendEntries, InstallSnapshot) are dispatched to it.
// Must be called after raft.Make() returns.
func (pn *PeerNetwork) RegisterRaft(rf raftapi.Raft) {
	pn.rpcs.AddService(rf)
}

// Ends returns the slice of ClientEnds to pass into raft.Make(peers, ...).
func (pn *PeerNetwork) Ends() []*labrpc.ClientEnd {
	return pn.ends
}

// Close shuts down the RPC server and all outbound connections.
func (pn *PeerNetwork) Close() {
	pn.rpcs.Close()
	for _, lc := range pn.lazies {
		if lc != nil {
			lc.Close()
		}
	}
}

// MakePeer creates a DiskPersister and PeerNetwork for one node.
// The caller should then call raft.Make(pn.Ends(), me, dp, applyCh)
// and pass the result to pn.RegisterRaft(rf).
func MakePeer(me, n int, dataDir string) (*PeerNetwork, *DiskPersister) {
	dp := MakeDiskPersister(dataDir)
	pn := NewPeerNetwork(me, n)
	log.Printf("peer %d/%d listening on %s, data in %s",
		me, n, sockrpc.SockName(peerName(me)), dataDir)
	return pn, dp
}

// MakePeerWithRaft is a convenience wrapper that creates the network and
// persister, calls makeRaft to start Raft, and registers the result.
func MakePeerWithRaft(
	me, n int,
	dataDir string,
	applyCh chan raftapi.ApplyMsg,
	makeRaft func(peers []*labrpc.ClientEnd, me int, persister *DiskPersister, applyCh chan raftapi.ApplyMsg) raftapi.Raft,
) (*PeerNetwork, raftapi.Raft) {
	pn, dp := MakePeer(me, n, dataDir)
	rf := makeRaft(pn.Ends(), me, dp, applyCh)
	pn.RegisterRaft(rf)
	return pn, rf
}
