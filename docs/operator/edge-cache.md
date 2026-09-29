# Edge cache operations (P08-T06)

Public reads cache at the edge; identity, location and private
payloads never do. The origin owns `Cache-Control`/ETag semantics;
the edge only stores what the origin marks shared, keyed on
method+URL, and purges on moderation or schedule.

## Allowlist: what caches

| Route | Cache-Control | ETag/304 | Notes |
|---|---|---|---|
| `GET /v1/stations?...` | `public, max-age=60` | yes | Non-sensitive queries only |
| `GET /v1/stations/{id}` | `public, max-age=30, s-maxage=60` | yes | Provenance/quality metadata |
| `GET /v1/stations/{id}/prices` | `public, max-age=60` | yes | Community section carries `expires_at`; clients must honor projection expiry over the TTL |
| `GET /v1/stations/{id}/official-prices?...` | `public, max-age=60` | yes | Revision-aware history |
| `GET /v1/stations/nearby?...` | `no-store` | n/a | Request position never cached, logged or echoed |
| All writes/owner reads/uploads/exports | `no-store` | n/a | Signed and owner-scoped |
| All error envelopes | `no-store` | n/a | Owner data never sits in shared caches |

DISPUTED/UNKNOWN projections render null amounts (never an ANP
fill-in), so a cached disputed price cannot mislead: there is no
price in it. Expired projections serve `STALE`/excluded-from-ranking
at query time regardless of worker health (B-BR-008).

## Key policy

- Key = method + URL only. Caddy strips `Cookie` and `Authorization`
  on the four shared-cacheable shapes (invariance proven by the
  `*IgnoreIdentity` unit tests: identical bodies with and without
  identity headers, no `Vary` on identity).
- Never add contributor, station or observation IDs to cache keys or
  log fields; purge procedures take URL prefixes, never identities.

## CDN rules (Cloudflare, provisioned at deploy)

1. Cache `GET /v1/stations*` except `/v1/stations/nearby*`; honor
   origin `Cache-Control` (bounded 30–60 s); bypass cache when the
   origin says `no-store`.
2. Do not vary on `Cookie`/`Authorization` for cached paths (origin
   already strips them on the shared set).
3. TLS 1.2+ to origin with strict origin authentication; trusted
   proxy ranges only (mirrors `Caddyfile`).
4. CDN outage serves from origin, never from a disabled-security
   shortcut.

## Purge procedure

Moderation actions, dispute resolutions and projection rebuilds that
change public prices purge the affected URL prefixes:

```sh
# Purge by URL prefix after a moderation decision (operator host)
curl -sS -X POST "https://api.cloudflare.com/client/v4/zones/${CF_ZONE}/purge_cache" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{"prefixes":["https://'"${CANONICAL_HOST}"'/v1/stations/<station_id>/"]}'
```

Bounded TTLs (≤60 s) cap any missed purge window; `expires_at` on
community sections bounds semantic staleness independently.

## Signature through proxy

Signed owner writes verify through the proxy unchanged: the proxy
forwards method/authority/path/query/content headers verbatim and
only strips `Cookie`/`Authorization` on the shared-cacheable GET
set (which carries no signed traffic). The regression suite below
covers headers; protocol vectors cover signatures (P03-T01, frozen).

## Rollback

Disable the CDN cache rule, purge the affected prefixes, and fall
back to secured origin reads. Origin `Cache-Control` stays correct
without the edge, so reads degrade to slower but safe.

## Regression suite

```sh
# Against a live origin or staging edge (read-only GETs + one 304 round-trip)
BASE_URL=https://staging.example.invalid bash scripts/cache-regression.sh
```

The suite asserts the allowlist matrix above, ETag/304 round-trips,
`no-store` on nearby/private/errors, identity invariance, and that
no response varies on `Cookie`/`Authorization`. Live edge runs happen
at staging deploy; the same matrix runs as unit tests in CI.
