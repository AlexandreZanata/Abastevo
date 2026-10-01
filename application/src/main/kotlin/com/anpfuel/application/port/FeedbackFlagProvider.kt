package com.anpfuel.application.port

/** P17-T01 — Feature flag for shared feedback flows. */
interface FeedbackFlagProvider {
    fun isEnabled(): Boolean
}
