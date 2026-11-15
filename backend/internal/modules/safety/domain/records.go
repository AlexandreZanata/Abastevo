package domain

import "time"

// Block is the owner's private view of one active block (B-BR-SAFE-07):
// never the blocked account id, never content text.
type Block struct {
	ID         string
	Label      string
	SourceKind ContentKind
	CreatedAt  time.Time
}

// BlockResult is a block command outcome; Replayed marks an existing
// block for the same account (B-BR-SAFE-05).
type BlockResult struct {
	Block    Block
	Replayed bool
}

// ReportReceipt is the reporter's receipt for a user report
// (B-BR-SAFE-04); Replayed marks the first report of the same pair.
type ReportReceipt struct {
	ID        string
	Reason    ReportReason
	CreatedAt time.Time
	Replayed  bool
}
