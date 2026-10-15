package domain

// ReviewDecision changes the visibility of a user claim, never official facts
// or chemical/purchase verification. Reasons are closed codes, not private text.
type ReviewDecision struct {
	Status    string
	ClearText bool
}

func DecideReportReview(action, reason string) (ReviewDecision, error) {
	switch {
	case action == "REVIEW" && reason == "TRIAGE":
		return ReviewDecision{}, nil
	case action == "RESOLVE" && reason == "CLAIM_RECORDED":
		return ReviewDecision{Status: "REVIEWED"}, nil
	case action == "DISMISS" && (reason == "DUPLICATE" || reason == "INSUFFICIENT_EVIDENCE" || reason == "UNSUPPORTED_LINK"):
		return ReviewDecision{Status: "HIDDEN"}, nil
	case action == "INVALIDATE" && (reason == "ABUSE" || reason == "PRIVACY"):
		return ReviewDecision{Status: "HIDDEN", ClearText: reason == "PRIVACY"}, nil
	default:
		return ReviewDecision{}, ErrReportInvalid
	}
}

type SignalClaim struct {
	Owner                  string
	Status                 string
	OwnerActive            bool
	OfficialLinkSuperseded bool
}

type PrivateSignal struct {
	ReviewedAccounts       int    `json:"reviewed_accounts"`
	DuplicateAccountClaims int    `json:"duplicate_account_claims"`
	Pending                int    `json:"pending"`
	Excluded               int    `json:"excluded"`
	Publication            string `json:"publication"`
	Independence           string `json:"independence"`
}

// ReviewOnlySignal deliberately cannot certify independent humans or purchases
// from account multiplicity. Its projection is restricted to operators.
func ReviewOnlySignal(claims []SignalClaim) PrivateSignal {
	out := PrivateSignal{Publication: "REVIEW_ONLY", Independence: "UNPROVEN"}
	owners := make(map[string]bool)
	for _, c := range claims {
		if c.Owner == "" || !c.OwnerActive || c.OfficialLinkSuperseded {
			out.Excluded++
			continue
		}
		switch c.Status {
		case "PENDING_REVIEW":
			out.Pending++
		case "REVIEWED":
			if owners[c.Owner] {
				out.DuplicateAccountClaims++
			} else {
				owners[c.Owner] = true
				out.ReviewedAccounts++
			}
		default:
			out.Excluded++
		}
	}
	return out
}
