# ADR-003: Nominatim reverse geocoding for optional device location (UC-012)

> **Status:** Accepted  
> **Date:** 2026-06-21  
> **Deciders:** Architecture / data layer  
> **Refs:** BR-021, UC-012, UC-003

## Context

UC-012 lets first-time users optionally skip manual state/city selection by resolving municipality from device GPS. We need a reverse geocoding provider that:

- Works for Brazilian municipalities.
- Requires no API key or paid backend (v3 scope: no app server).
- Can be invoked at most once per onboarding session per user (low volume).

Alternatives considered:

| Option | Pros | Cons |
|--------|------|------|
| **Public Nominatim API** | Free, no key, good OSM coverage in Brazil | Strict rate limits; shared resource; requires attribution |
| **Self-hosted Nominatim** | No external rate limit | Ops burden, storage, not justified for one-shot onboarding |
| **Google Geocoding API** | High accuracy | API key, billing, privacy policy change |
| **Android Geocoder** | Built-in | Inconsistent on devices; still needs network |

## Decision

Use the **public Nominatim reverse geocode API** at `nominatim.openstreetmap.org` with strict client-side compliance (BR-021):

1. **Custom `User-Agent`** identifying app name and version (contact email in UA string).
2. **Rate limit:** max 1 request per second; single-threaded client.
3. **Cache:** persist successful `(lat, lon)` rounded to 3 decimal places → `ReverseGeocodeResult` in local DataStore.
4. **Scope:** only user-triggered onboarding flow — no background periodic geocoding.
5. **Privacy:** do not persist raw GPS coordinates; only resolved municipality + state via UC-003.
6. **Attribution:** display OSM/Nominatim attribution on location prompt and in settings.

Fallback: UC-003 manual picker on any failure.

## Consequences

### Positive

- No backend or API keys.
- Aligns with open-source / privacy-first product model.
- One-shot usage stays well within Nominatim fair-use policy.

### Negative

- Dependency on third-party availability.
- Must monitor Nominatim policy changes; may require ADR revision to self-host if user base grows.

### Follow-up

- If blocked or rate-limited in production, evaluate Photon self-host or local Nominatim Docker for Brazil extract only.

## Compliance checklist (implementation)

- [ ] OkHttp interceptor sets non-default User-Agent
- [ ] `NominatimRateLimiter` enforces 1 req/s
- [ ] Geocode cache before network
- [ ] No WorkManager periodic geocode jobs
- [ ] i18n attribution strings (`geocoding_osm_attribution`)
- [ ] Privacy policy updated
