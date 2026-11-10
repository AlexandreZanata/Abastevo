package com.anpfuel.application.usecase.legal

import com.anpfuel.application.port.TermsAcceptanceStore
import com.anpfuel.domain.legal.LegalDocuments
import com.anpfuel.domain.legal.TermsAcceptance
import com.anpfuel.domain.legal.TermsAcceptanceRule
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

sealed interface TermsStatus {
    /** [changed] is true when an earlier acceptance exists for other document versions. */
    data class Required(val documents: LegalDocuments, val changed: Boolean) : TermsStatus
    data class Accepted(val record: TermsAcceptance) : TermsStatus
}

/**
 * First-use terms gate (B-BR-TA01..TA06). One instance per process: the gate
 * and the Privacy screen observe the same [status], so a revocation returns to
 * the terms screen at once. Nothing here grants the optional training consent.
 */
class TermsAcceptanceUseCase(
    private val store: TermsAcceptanceStore,
    private val documents: LegalDocuments,
    private val nowMillis: () -> Long = System::currentTimeMillis,
) {
    private val mutable = MutableStateFlow(read())
    val status: StateFlow<TermsStatus> = mutable.asStateFlow()

    /** False when the box is not ticked or the record could not be stored; the gate stays. */
    @Synchronized
    fun accept(ticked: Boolean): Boolean {
        if (!ticked) return false
        val record = TermsAcceptanceRule.accept(documents, ticked = true, nowMillis = nowMillis())
        val stored = runCatching { store.save(record) }.getOrDefault(false)
        mutable.value = read()
        return stored && mutable.value == TermsStatus.Accepted(record)
    }

    /** False when the record could not be deleted; the state then still reflects the disk. */
    @Synchronized
    fun revoke(): Boolean {
        val cleared = runCatching { store.clear() }.getOrDefault(false)
        mutable.value = read()
        return cleared && mutable.value is TermsStatus.Required
    }

    private fun read(): TermsStatus {
        val record = runCatching { store.load() }.getOrNull()
        return if (TermsAcceptanceRule.isCurrent(record, documents, nowMillis())) {
            TermsStatus.Accepted(record!!)
        } else {
            val olderVersion = record != null && record.acceptedAtMillis > 0 &&
                (record.termsVersion != documents.termsVersion || record.privacyVersion != documents.privacyVersion)
            TermsStatus.Required(documents, changed = olderVersion)
        }
    }
}
