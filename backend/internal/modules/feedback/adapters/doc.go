// Package adapters owns the Postgres Store for station/fuel ratings
// (P14-T02). Rating writes lock the live key row and refresh stats in
// the same transaction; a lost unique race re-reads the winner so
// concurrent writers converge instead of forking.
package adapters
