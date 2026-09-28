// Package adapters implements the official Store port on pgx (P02-T06). It
// owns the generated official queries package; no other module may import
// it. Small UUID helpers are intentionally duplicated from
// directory/adapters instead of imported: adapters never cross-import.
// Publication is one transaction (validate, mark, switch pointer) so a
// partial batch can never become current; staged rows of failed or
// under-review revisions stay unreachable because only the pointer grants
// visibility.
package adapters
