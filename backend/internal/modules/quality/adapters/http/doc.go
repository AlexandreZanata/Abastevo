// Package http serves anonymous quality ledger reads under /v1/quality.
// Every response carries its frozen lease (stream, edition, revision,
// denominators, periods, staleness); source CNPJs and internal notes
// never reach the wire. A disabled switch answers 404 without touching
// storage; storage failures answer 503 without internals.
package http
