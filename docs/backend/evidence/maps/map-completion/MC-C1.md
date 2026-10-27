# MC-C1 Brazil base-map generation (regional packages)

Status: generation complete on local disk (23/23 regional packages present);
signed distribution, staging serving proof, World/WorldCoasts compatibility
and device validation remain pending. Date: 2026-10-27.
Base: `394c1389` plus this task. Supervision/evidence time: ~30 minutes;
failed first West attempt ran 2,298.64 s before the routing-stage timeout, and
the successful West retry took ~2,227 s launch-to-package.

## Scope recorded here

All 23 `Brazil_*` regions were generated with the current
`scripts/maps/generate-regional-map.sh`, `--borders 'Brazil_*'` and the pinned
CoMaps toolchain. Packages stay on local disk under
`/data/dev/src/comaps-brazil/mwm/`; no `.mwm` bytes are committed.
The per-region border setup fell from ~632 s to 14–23 s after the script fix;
the times below are full per-region script times, still dominated by
features/geometry/search/routing.

The `check_mwm` diagnostic exits non-zero for these packages; the per-package
receipts record `check_mwm_exit: 134`. This is the known strict-checker
diagnostic, not engine acceptance.

## West retry

The v2 batch left `Brazil_Northeast Region_West` as `FAIL` after `TIME
2298.64` / `RSS 1837984` because the routing stage exceeded `--timeout 1800`.
The stale work directory was preserved under `old-run-20261027/`, and the
region was regenerated alone with `--timeout 3600 --replace`.

Retry result: package `Brazil_Northeast Region_West.mwm`, 185,957,500 bytes,
SHA-256 `021f0cabeeb44c842eb1c00d9bf1f25c58c3f81215eaee3cbd8c1f539a23d420`,
`check_mwm_exit: 134`. Duration was measured launch-to-package as ~2,227 s;
peak RSS was not captured for the retry wrapper.

## Regional packages

| Region | Result | Bytes | SHA-256 | Time | Peak RSS |
| --- | --- | ---: | --- | ---: | ---: |
| Brazil_Bahia | OK | 102687418 | `23f34c3411017481a71d76e87bab3ee3ece5b6b25557937e43c9f5110df7cb1b` | 1618.88 s | 1173188 |
| Brazil_Goias_Brasilia | OK | 74658071 | `102ce8cc550fc92e2e7bbf20e71b8553f199a63a225cc431646e3bd3c5cc3818` | 634.98 s | 966068 |
| Brazil_Goias_North | OK | 21017137 | `706f1cb97ee584c6b4df0dabc8e095ab3012a92fe162c21c91306be0c62109fe` | 272.76 s | 489768 |
| Brazil_Mato Grosso Do Sul | OK | 29236213 | `ca09efb9779f0819442d1a4337702d8234d3697f9d887f5a7a706c9ede6dbcb1` | 282.24 s | 559848 |
| Brazil_Mato Grosso | OK | 53816563 | `3f908fd92121b0b2cc7016c8ec3aa3d7befa81f4199ee6b33759a70aae8a85e1` | 243.89 s | 722172 |
| Brazil_North Region_East | OK | 73260727 | `0882389ecfc9d1dbabca041b14412ab8d62e6029040cd421f1322f1a33674f7f` | 588.58 s | 856176 |
| Brazil_North Region_West | OK | 87357592 | `a22a50e222df9531214f68170b2290ae09e1dda0eb95b65b2fdd950c8593dd2b` | 681.14 s | 1166232 |
| Brazil_Northeast Region_East | OK | 71165417 | `bc806c54934331ab6e4974ffe52ac50b057f595edb6c0765533aa281e00ec9f7` | 806.55 s | 856808 |
| Brazil_Northeast Region_West | OK on retry | 185957500 | `021f0cabeeb44c842eb1c00d9bf1f25c58c3f81215eaee3cbd8c1f539a23d420` | ~2227 s | not captured |
| Brazil_Paraiba | OK | 34651529 | `405ac5aaf655b784c422a8008e9cd02f098248a8136bb8376dd646ef4819e802` | 236.54 s | 548060 |
| Brazil_Parana_East | OK | 56658777 | `4e7e6b7d4d4b67f6488182639280c625228f2aa09d010f89447ef461b70922fb` | 598 s | not captured |
| Brazil_Parana_West | OK | 86577193 | `3d091ada682100a18e0fe763298c54864dc13773e4078942c5c8e0e4897d3ca6` | 753.02 s | 1078232 |
| Brazil_Rio Grande do Norte | OK | 19821080 | `aef814ac10defd7a9990b613f803b055317a4a94680a2ec203e405762aafdc8a` | 192.87 s | 447864 |
| Brazil_Santa Catarina | OK | 115913667 | `0b856db1b1e4f775da18f4179697cd9e365d89548e5a0ed459ee0b4160627169` | 1557.54 s | 1318296 |
| Brazil_South Region_East | OK | 77672183 | `6938a93a18d83c0afa1353ec831123dbf1a3adebeb208e55a9f94bbf911b6f44` | 726.17 s | 925720 |
| Brazil_South Region_West | OK | 55086115 | `286727af619633eaaa96818fc27cd607cc351fabc4df3b17982c71081e7e7f5e` | 494.48 s | 848908 |
| Brazil_Southeast Region_Espirito Santo | OK | 197373933 | `47bc14b1c88bf6c6cf69856a22056e66e87ab2b788251b36bbb3ab49c99c89d1` | 2237.12 s | 1883052 |
| Brazil_Southeast Region_Minas Gerais_Contagem | OK | 131772382 | `3acbbb9dadb3eb96487ee3e894fb7c2108ceb2a5ae850deff1d2232004ee26d7` | 2628.72 s | 1274608 |
| Brazil_Southeast Region_Minas Gerais_North | OK | 103720652 | `0f5ec7684e8c4d734741b326c0f078a8e6e2a75f96ef930e50cb5c716162ab3a` | 1066.77 s | 1274608 |
| Brazil_Southeast Region_Rio de Janeiro | OK | 67708310 | `0a51f55726443257df8551404fc2cdee0e9846beb0366164a5567770d1dd2650` | 946.11 s | 1006920 |
| Brazil_Southeast Region_Sao Paulo_Campinas | OK | 92189242 | `4a723b8adac52bffa439caed39cf3e488b3e7e22e9c554b3f1d434e228c5a74d1` | 1388.15 s | 1036368 |
| Brazil_Southeast Region_Sao Paulo_City | OK | 151947666 | `71db0e143fa6a1bff35528ce9267b49846cae4175eb049f76ad7a0d1cad44b12` | 1640.89 s | 1524600 |
| Brazil_Southeast Region_Sao Paulo_West | OK | 63441905 | `fba5271de325339fafb3e3eba5869780ae3c3e8073f44b5e08c811cd3c962e25` | 381.90 s | 736132 |

Total: 23 packages, 1,953,691,272 bytes (~1.82 GiB).

## Still pending in MC-C1

- World/WorldCoasts compatibility check or toolchain generation.
- Signed distribution envelope and static serving proof (ETag/304, Range resume,
  tampered package refusal).
- Device validation of the generated packages.
