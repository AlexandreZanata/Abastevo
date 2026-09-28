# ADR-007: Anonymous proof and mobile key compatibility

Status: **Proposed protocol; freeze in P03**. Date: 2026-09-28. Scope: planned platform; implementation evidence required by roadmap gates.

## Context

No traditional login is required; Ed25519 secure-keystore availability cannot be assumed for all minSdk 26 devices.

## Decision

Recommend P-256/SHA-256 with Android Keystore and RFC 9421 profile, nonce challenges, bound proof, durable replay/idempotency, rotation with both key proofs. Anonymous key loss cannot recover identity. Verify interoperability/security vectors P03 and actual Android device support P10.

## Alternatives

Ed25519 after support evidence; bearer-only anonymous token; mandatory Google/email account.

## Consequences

Extra challenge roundtrip and signature/proxy validation complexity; no unique-human guarantee. Use existing cryptographic primitives; protocol/library selection requires reviewed vectors before release.

## Validation and follow-up

Use the owning tasks in [ROADMAP](../../ROADMAP.md), specifications in [docs index](../README.md) and [decision log](../planning/DECISIONS.md). Record tested policy/tool versions before marking a release gate complete.
