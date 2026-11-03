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

## Pinned source recovery opening

B-BR-EV-SRC-01. An HTTP content coding must not change the admitted IBGE
document bytes or bypass the acquisition byte ceiling.

BUC-EVC-SRC-RECOVERY. Recover the previously authorized IBGE edition
when the publisher applies gzip. Refuse corrupted and over-budget
compressed responses before writing a source file.

Status: LOCAL_DONE
Validation: PASS

The official IBGE endpoint returned gzip with HTTP 200 and the old
collector rejected the compressed body as invalid JSON. The collector
now bounds both transport and decoded bytes for this endpoint and
refuses corrupt gzip. Eight acquisition test groups passed. A fresh
authorized retrieval produced 3,998,504 decoded bytes with the exact
EVC-01 SHA-256 5491bd548fe381cd388ba81c9edfbf4cf2749891e2fd32c1079ac71f7aba1c10.
No source edition changed.
