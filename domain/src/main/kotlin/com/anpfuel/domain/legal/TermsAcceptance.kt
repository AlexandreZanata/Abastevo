package com.anpfuel.domain.legal

/** Versions of the public Terms of Use and Privacy Policy a person accepts before using the app (B-BR-TA03). */
data class LegalDocuments(val termsVersion: String, val privacyVersion: String) {
    init {
        require(validVersion(termsVersion) && validVersion(privacyVersion)) { "legal.version" }
    }
}

/** The whole first-use record (B-BR-TA02): two versions and the instant, nothing about the person. */
data class TermsAcceptance(val termsVersion: String, val privacyVersion: String, val acceptedAtMillis: Long)

/** First-use acceptance rule. Never a consent for optional processing (B-BR-TA05). */
object TermsAcceptanceRule {
    /** A device clock corrected backwards by a few minutes must not lock the person out. */
    const val CLOCK_SKEW_MILLIS = 10 * 60 * 1000L

    fun accept(documents: LegalDocuments, ticked: Boolean, nowMillis: Long): TermsAcceptance {
        require(ticked) { "legal.not-ticked" }
        require(nowMillis > 0) { "legal.instant" }
        return TermsAcceptance(documents.termsVersion, documents.privacyVersion, nowMillis)
    }

    fun isCurrent(record: TermsAcceptance?, documents: LegalDocuments, nowMillis: Long): Boolean =
        record != null &&
            record.termsVersion == documents.termsVersion &&
            record.privacyVersion == documents.privacyVersion &&
            record.acceptedAtMillis > 0 &&
            record.acceptedAtMillis <= nowMillis + CLOCK_SKEW_MILLIS
}

internal fun validVersion(value: String): Boolean =
    value.length in 1..32 && value.all { it in '!'..'~' }

/**
 * Identifiers printed on the public documents (B-BR-TA03): the Terms of Use
 * "Última atualização" month and the Privacy Policy version. Change them
 * together with `landing/static/termos` and `landing/static/privacidade`;
 * `PublishedLegalDocumentsTest` compares both.
 */
object PublishedLegalDocuments {
    val CURRENT = LegalDocuments(termsVersion = "2026-10", privacyVersion = "1.0")
}
