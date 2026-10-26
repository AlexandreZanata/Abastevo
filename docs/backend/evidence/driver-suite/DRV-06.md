# DRV-06 — Vehicle cost profiles and exact allocation

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Tested behavior: `d21e5a34`. Contract: [DRV-03–07](../../../product/DRIVER_03_07_CONTRACT.md).

T01: dated/versioned owned, leased and financed profiles retain source ledger
identity/revision. Principal is capital/cash; interest/lease is operating actual;
depreciation/reserve stays modeled. Unknown assumptions remain unknown. Duplicate
ledger sources and leased depreciation/financing combinations refuse.
T02: Go/Kotlin exact largest remainder with bounded stable ASCII IDs allocates
100 centavos as a34/b33/c33; sums and permutations tested for 0..100 and maximum
integer products. Time/civil-day proration retains an unallocated remainder,
refuses overlapping subwindows and counts the March DST day as a calendar day.
T03: explicit one-to-one same-window service/reserve replacement keeps actual
money once and removes the linked modeled addition; correction 2000→3000 does
not restore/subtract the reserve. Missing assumptions and bad links refuse or
remain unknown. No default market/tax/depreciation rates or duplicate ledger
entry is created.

RED `/tmp/drv06-red.log`: 100 split into 33+33+33 lost a centavo. GREEN
`/tmp/drv06-kotlin.log` (5 tests), Go driver-domain race suite/vet
(`/tmp/drv06-go.log`), all PASS. Original financial oracle expectations remain
unchanged. Diff/secret checks PASS. Pure rules/profile model only; cost-profile
persistence and driver reports belong to later phases.


No deployment, provider activation or suite/UI release claim. Next DRV-07.
