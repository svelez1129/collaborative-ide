package ide

// CRDTUpdateCmd is submitted to Raft when a client sends a Yjs update
type CRDTUpdateCmd struct {
	Update []byte // raw Yjs binary update
}

// ProposalCmd is submitted to Raft when a proposer suggests a change,
// or when an editor accepts/rejects one
type ProposalCmd struct {
	ID          string // unique proposal ID
	Author      string // userID of proposer
	StartIndex  int    // start char index in document
	EndIndex    int    // end char index in document
	Replacement string // proposed replacement text
	Action      string // "add" | "accept" | "reject"
}

// RoleChangeCmd is submitted to Raft when an editor changes someone's role
type RoleChangeCmd struct {
	UserID string // target user
	Role   string // "editor" | "proposer" | "viewer"
}
