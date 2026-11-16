// Package runtimegrants derives the privilege inventory of the ordinary
// runtime login that serves the composed API and worker (EMP-REL-002,
// B-BR-EMP-556..558, EDR-363) and renders the reviewed, idempotent
// provisioning script infra/db/runtime-role.sql.
//
// The inventory is the union of what the production binaries' SQL does to
// each relation, what the migrated head's SECURITY INVOKER functions, RLS
// policies, defaults and checks do when they run as that login, and an
// explicit reviewed exception list. Nothing here connects to a deployment or
// provisions a live role; operators apply the rendered script.
package runtimegrants

import (
	"regexp"
	"strings"
)

// Usage accumulates, per lower-case identifier, what runtime SQL does with
// it. Identifiers are resolved against the catalog only when an inventory is
// built, so non-relation words collected here never become grants.
type Usage struct {
	Mentioned map[string]bool
	Insert    map[string]bool
	Update    map[string]bool
	Delete    map[string]bool
	Calls     map[string]bool
}

// NewUsage returns an empty accumulator.
func NewUsage() *Usage {
	return &Usage{Mentioned: map[string]bool{}, Insert: map[string]bool{}, Update: map[string]bool{}, Delete: map[string]bool{}, Calls: map[string]bool{}}
}

// Merge adds every entry of other to u.
func (u *Usage) Merge(other *Usage) {
	for _, pair := range []struct{ to, from map[string]bool }{{u.Mentioned, other.Mentioned}, {u.Insert, other.Insert}, {u.Update, other.Update}, {u.Delete, other.Delete}, {u.Calls, other.Calls}} {
		for name := range pair.from {
			pair.to[name] = true
		}
	}
}

const identifier = `(?:"?public"?\s*\.\s*)?"?([A-Za-z_][A-Za-z0-9_$]*)"?`

var (
	lineComment  = regexp.MustCompile(`--[^\n]*`)
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	sqlLike      = regexp.MustCompile(`(?i)\b(?:SELECT|INSERT|UPDATE|DELETE|FROM|JOIN|WITH|PERFORM|RETURNING)\b`)
	word         = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_$]*`)
	insertInto   = regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+(?:ONLY\s+)?` + identifier)
	updateWord   = regexp.MustCompile(`(?i)\bUPDATE\b`)
	updateTarget = regexp.MustCompile(`(?i)^UPDATE\s+(?:ONLY\s+)?` + identifier + `(?:\s+(?:AS\s+)?[A-Za-z_][A-Za-z0-9_]*)?\s+SET\b`)
	deleteFrom   = regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+(?:ONLY\s+)?` + identifier)
	onConflict   = regexp.MustCompile(`(?i)\bON\s+CONFLICT\b`)
	doUpdate     = regexp.MustCompile(`(?i)\bDO\s+UPDATE\b`)
	rowLock      = regexp.MustCompile(`(?i)\bFOR\s+(?:NO\s+KEY\s+UPDATE|UPDATE|KEY\s+SHARE|SHARE)\b`)
	fromOrJoin   = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+(?:ONLY\s+)?` + identifier)
	call         = regexp.MustCompile(`(?:"?public"?\s*\.\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	precedingKey = regexp.MustCompile(`(?i)\b(DO|FOR|KEY|ON|OR|BEFORE|AFTER|OF)\s*$`)
)

// AddSQL records one SQL text (one or more statements). Comments are removed;
// statements are split on semicolons so that row locks and ON CONFLICT
// updates apply only to the relations of their own statement.
func (u *Usage) AddSQL(text string) {
	text = blockComment.ReplaceAllString(lineComment.ReplaceAllString(text, " "), " ")
	if !sqlLike.MatchString(text) {
		return
	}
	for _, statement := range strings.Split(text, ";") {
		u.addStatement(statement)
	}
}

// AddExpression records a catalog expression evaluated as the calling login
// (a policy, a column default, a CHECK constraint, a view): every word is a
// possible relation or sequence reference and every call a function use.
func (u *Usage) AddExpression(text string) {
	for _, token := range word.FindAllString(text, -1) {
		u.Mentioned[strings.ToLower(token)] = true
	}
	for _, m := range call.FindAllStringSubmatch(text, -1) {
		u.Calls[strings.ToLower(m[1])] = true
	}
}

func (u *Usage) addStatement(statement string) {
	if !sqlLike.MatchString(statement) {
		return
	}
	for _, token := range word.FindAllString(statement, -1) {
		u.Mentioned[strings.ToLower(token)] = true
	}
	inserted := map[string]bool{}
	for _, m := range insertInto.FindAllStringSubmatch(statement, -1) {
		name := strings.ToLower(m[1])
		u.Insert[name] = true
		inserted[name] = true
	}
	for _, at := range updateWord.FindAllStringIndex(statement, -1) {
		if precedingKey.MatchString(statement[:at[0]]) {
			continue
		}
		if m := updateTarget.FindStringSubmatch(statement[at[0]:]); m != nil {
			u.Update[strings.ToLower(m[1])] = true
		}
	}
	for _, m := range deleteFrom.FindAllStringSubmatch(statement, -1) {
		u.Delete[strings.ToLower(m[1])] = true
	}
	if onConflict.MatchString(statement) && doUpdate.MatchString(statement) {
		for name := range inserted {
			u.Update[name] = true
		}
	}
	if rowLock.MatchString(statement) {
		// A row lock needs UPDATE on the locked relation; the locked set is
		// over-approximated by every relation the statement reads.
		for _, m := range fromOrJoin.FindAllStringSubmatch(statement, -1) {
			u.Update[strings.ToLower(m[1])] = true
		}
	}
	for _, m := range call.FindAllStringSubmatch(statement, -1) {
		u.Calls[strings.ToLower(m[1])] = true
	}
}
