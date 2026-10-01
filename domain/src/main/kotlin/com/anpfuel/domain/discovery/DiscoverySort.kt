package com.anpfuel.domain.discovery

/**
 * P20-T01 — Bounded discovery sort.
 *
 * Only sorts computable without precise device GPS. Distance sorting stays
 * out of this contract until P20-T02 proves location integrity (B-BR-C06).
 */
enum class DiscoverySort {
    PRICE_ASC,
    RECENCY_DESC,
}
