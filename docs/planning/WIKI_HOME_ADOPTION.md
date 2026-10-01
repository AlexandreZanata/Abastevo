# Wiki Home adoption — explicit one-time authorization

The maintainer explicitly requested on 2026-10-01 that wiki Home use the README banner and organization. Previously root README was mirrored to Project-Overview.md; manual Home was a one-line welcome and was not owned.

Expected old Home SHA256: `1543cd2d394ea3fa1f894046f51cd0d89f942aa79261d534c3bcfb61f6f677c8`. Original content is preserved in [history](history/WIKI_HOME_BEFORE_20261001.md) and wiki Git history. Source config now selects Home.md.

Publication must first independently verify repository ID 1393052962 and enabled wiki, trusted wiki remote (the former brazil-fuel-prices alias resolves to canonical AlexandreZanata/abastevo), current remote head and a clean checkout. Compare the actual Home bytes to the expected hash; if it changed, stop and reconcile rather than overwrite. Add only Home ownership with that exact hash in a local manifest-adoption commit. This explicit adoption is not general permission to take over other manual pages.

Run the existing wiki publisher from the verified merged source SHA against that clean wiki checkout, with --allow-publish. It applies ordinary content-hash guards, mirrors README/banner/navigation to Home, removes only unchanged obsolete owned Project-Overview and preserves unrelated manual pages. Push adoption and sync commits in one final wiki push; no intermediate remote adoption. Check immutable banner URL, Home content, manifest source SHA, manual-page equality and remote head. Failure remains WIKI_PENDING, not another backend test run.
