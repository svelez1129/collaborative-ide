package ide

import (
	"fmt"
	"log"
	"net/http"

	"github.com/svelez1129/collaborative-ide/src/rsm"
)

// MainSingle runs a single-node server with no Raft consensus.
// Useful for local development and testing the IDE features.
func MainSingle(port string) {
	hub := NewHub()
	collab := NewCollabServer(hub)
	server := NewServer(hub, collab, nil)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	fmt.Printf("GoCollab IDE (single-node) running at http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// MainCluster runs one node of a multi-node fault-tolerant cluster.
//
//	me       – this node's index (0-based)
//	nodes    – total number of Raft peers
//	basePort – HTTP port for peer 0; peer i listens on basePort+i
//	dataDir  – directory for this peer's DiskPersister files
//
// Start a 3-node cluster by running three processes:
//
//	go run cmd/ide/main.go --mode cluster --me 0 --nodes 3 --base-port 8080
//	go run cmd/ide/main.go --mode cluster --me 1 --nodes 3 --base-port 8080
//	go run cmd/ide/main.go --mode cluster --me 2 --nodes 3 --base-port 8080
//
// Clients always connect to the leader. Non-leader nodes send a
// {"type":"redirect","port":N} frame so the browser reconnects to the right node.
func MainCluster(me, nodes int, basePort int, dataDir string) {
	// Create the peer network (Unix socket RPC) and disk persister.
	pn, dp := MakePeer(me, nodes, fmt.Sprintf("%s/peer-%d", dataDir, me))

	// Wire everything together: hub, sessions, state machine, RSM.
	hub := NewHub()
	collab := NewCollabServer(hub)

	// MakeRSM calls raft.Make internally and starts the reader goroutine.
	// We then register the Raft peer with the PeerNetwork so incoming RPCs
	// (RequestVote, AppendEntries, InstallSnapshot) are dispatched to it.
	r := rsm.MakeRSM(pn.Ends(), me, dp, -1, collab)
	pn.RegisterRaft(r.Raft())

	// Restore any snapshot that was on disk so the state machine is current.
	if snap := dp.ReadSnapshot(); len(snap) > 0 {
		collab.Restore(snap)
	}

	server := NewServer(hub, collab, r)
	// leaderPort is the base port; redirect adds the leader's peer index on top.
	server.leaderPort = basePort

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	httpPort := fmt.Sprintf("%d", basePort+me)
	fmt.Printf("GoCollab IDE peer %d/%d running at http://localhost:%s\n", me, nodes, httpPort)
	log.Fatal(http.ListenAndServe(":"+httpPort, mux))
}
