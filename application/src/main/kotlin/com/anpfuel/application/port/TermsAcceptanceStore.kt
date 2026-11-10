package com.anpfuel.application.port

import com.anpfuel.domain.legal.TermsAcceptance

/** Device-local first-use record (B-BR-TA02). A corrupt or partial record loads as null (B-BR-TA06). */
interface TermsAcceptanceStore {
    fun load(): TermsAcceptance?

    /** True only when the record is durably stored. */
    fun save(record: TermsAcceptance): Boolean

    /** True only when no record remains. */
    fun clear(): Boolean
}
