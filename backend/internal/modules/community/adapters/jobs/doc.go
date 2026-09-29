// Package jobs connects the durable validation handler (P04-T05): each
// validate-observation job claims its observation with the job ID as the
// persisted command proof, then runs admissibility through owned ports and
// persists the terminal transition plus the consensus intent. Unknown
// payload versions fail toward DEAD instead of running blind; pending
// evidence and unavailable signals retry without regressing state, and
// terminal replays converge instead of duplicating decisions.
package jobs
