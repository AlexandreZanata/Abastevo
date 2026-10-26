package com.anpfuel.domain.routing

/** Pure guidance guards: request binding, reroute timeout and native locale mapping. */
object GuidancePolicy {
    const val REROUTE_TIMEOUT_NANOS = 60_000_000_000L

    fun sameRoute(first: NativeRouteRequest, second: NativeRouteRequest): Boolean {
        if (first.points.size != second.points.size) return false
        if (first.avoid != second.avoid) return false
        return first.points.zip(second.points).all { (a, b) ->
            a.latitude == b.latitude && a.longitude == b.longitude
        }
    }

    fun isRerouteExpired(rerouteStartNanos: Long, nowNanos: Long): Boolean =
        nowNanos >= rerouteStartNanos && nowNanos - rerouteStartNanos > REROUTE_TIMEOUT_NANOS

    /** Explicit native turn-text locale; unknown app languages never become native codes. */
    fun nativeLocaleFor(language: String): String? {
        val normalized = language.trim().replace('_', '-')
        if (normalized.isEmpty()) return null
        val base = normalized.substringBefore('-').lowercase()
        val region = normalized.substringAfter('-', "").uppercase()
        return when {
            normalized.equals("pt-BR", ignoreCase = true) -> "pt-BR"
            normalized.equals("zh-Hans", ignoreCase = true) -> "zh-Hans"
            normalized.equals("zh-CN", ignoreCase = true) -> "zh-Hans"
            normalized.equals("zh-SG", ignoreCase = true) -> "zh-Hans"
            base == "pt" -> "pt-BR"
            base == "zh" -> if (region == "TW" || region == "HK" || region == "MO") null else "zh-Hans"
            base == "en" -> "en"
            base == "de" -> "de"
            base == "es" -> "es"
            base == "fr" -> "fr"
            base == "ja" -> "ja"
            base == "ru" -> "ru"
            else -> null
        }
    }
}
