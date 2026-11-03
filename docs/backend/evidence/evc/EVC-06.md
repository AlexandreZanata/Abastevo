# EVC-06 publication evidence

## EVC-06A opening

B-BR-EVC-06A. Compare counts of canonical sites in each Brazilian UF
against the last published generation. Refuse a drop strictly greater
than 15 percent in any UF, including complete disappearance. Unassigned
sites remain a separate diagnostic and never compensate for a UF drop.
An owner override is a separate publication decision.

BUC-EVC-06A. Check first publication, exact threshold, small counts,
disappearance and simultaneous loss/growth across UFs. Reject invalid
census input instead of treating it as a first publication.

Status: LOCAL_DONE
Validation: PASS

The initial tests failed on both missing-state coverage and invalid
input. The comparison now uses integer arithmetic with the exact
strict threshold, without overflow. State changes cannot be hidden
by gains elsewhere or by unassigned counts. Charging domain tests
with race detection, vet, formatting and secret review passed.
