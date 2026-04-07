package ide

// CRDTUpdateCmd is submitted to Raft when a client sends a Yjs update.
type CRDTUpdateCmd struct {
	SessionCode string
	Update      []byte // raw Yjs binary update
}

// ProposalCmd is submitted to Raft when a proposer suggests a change,
// or when an editor accepts/rejects one.
type ProposalCmd struct {
	SessionCode  string
	ID           string // unique proposal ID
	Author       string // userID of proposer
	Replacement  string // proposed replacement text
	OriginalText string // text that was selected (empty = insertion)
	RelStart     string // base64-encoded Yjs relative position
	RelEnd       string // base64-encoded Yjs relative position
	StartLine    int    // 1-based start line number (snapshot at submission)
	EndLine      int    // 1-based end line number (snapshot at submission)
	Action       string // "add" | "accept" | "reject"
}

// RoleChangeCmd is submitted to Raft when an editor changes someone's role.
type RoleChangeCmd struct {
	SessionCode string
	UserID      string // target user
	Role        string // "editor" | "proposer" | "viewer"
}
