//! Replay and delta decisions over versioned manifests. Identical source
//! bytes under identical parser/policy/alias versions are a no-op;
//! changed bytes publish as dated corrections; older evidence never
//! overwrites fresher facts.

use crate::output::{Manifest, SourceMeta};

/// Publication decision for freshly emitted inputs against the last
/// accepted manifest.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ReplayDecision {
    /// Same bytes under same versions: nothing to do.
    Noop,
    /// New or changed evidence under a current edition: publish.
    Publish,
    /// Current edition predates accepted evidence: refuse.
    Stale,
}

/// Decide publication. `parser_version`, `policy_version` and
/// `alias_digest` are the current run's; anything recorded differently
/// in `previous` forces a fresh evaluation (`Publish`).
pub fn replay_decision(
    previous: &Manifest,
    current_inputs: &[SourceMeta],
    parser_version: &str,
    policy_version: &str,
    alias_digest: &str,
) -> ReplayDecision {
    if previous.parser_version != parser_version
        || previous.policy_version != policy_version
        || previous.municipality_reference.reference_hash != alias_digest
    {
        return ReplayDecision::Publish;
    }
    let mut changed = false;
    for current in current_inputs {
        match previous
            .inputs
            .iter()
            .find(|input| input.key == current.key)
        {
            None => changed = true,
            Some(known)
                if known.sha256 != current.sha256
                    || known.bytes != current.bytes
                    || known.rows != current.rows =>
            {
                if current.edition_seq < known.edition_seq {
                    return ReplayDecision::Stale;
                }
                changed = true;
            }
            Some(known) if current.edition_seq != known.edition_seq => {
                changed = true;
            }
            _ => {}
        }
    }
    if changed || previous.inputs.len() != current_inputs.len() {
        ReplayDecision::Publish
    } else {
        ReplayDecision::Noop
    }
}
