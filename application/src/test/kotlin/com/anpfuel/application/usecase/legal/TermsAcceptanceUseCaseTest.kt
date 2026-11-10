package com.anpfuel.application.usecase.legal

import com.anpfuel.application.port.TermsAcceptanceStore
import com.anpfuel.domain.legal.LegalDocuments
import com.anpfuel.domain.legal.TermsAcceptance
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class TermsAcceptanceUseCaseTest {
    private val documents = LegalDocuments("2026-10", "1.0")
    private var now = 1_800_000_000_000L

    private class Disk(var record: TermsAcceptance? = null, var writable: Boolean = true, var readable: Boolean = true) : TermsAcceptanceStore {
        var saves = 0
        override fun load(): TermsAcceptance? = if (readable) record else error("disk unavailable")
        override fun save(record: TermsAcceptance): Boolean { saves++; if (writable) this.record = record; return writable }
        override fun clear(): Boolean { if (writable) record = null; return writable }
    }

    private fun useCase(disk: Disk) = TermsAcceptanceUseCase(disk, documents) { now }

    @Test fun `first use requires acceptance and a ticked accept opens the app with a record`() {
        val disk = Disk()
        val terms = useCase(disk)
        assertEquals(TermsStatus.Required(documents, changed = false), terms.status.value)
        assertTrue(terms.accept(ticked = true))
        val record = TermsAcceptance("2026-10", "1.0", now)
        assertEquals(TermsStatus.Accepted(record), terms.status.value)
        assertEquals(record, disk.record)
        assertEquals(TermsStatus.Accepted(record), useCase(disk).status.value)
    }

    @Test fun `accept without a tick records nothing and keeps the gate`() {
        val disk = Disk()
        val terms = useCase(disk)
        assertFalse(terms.accept(ticked = false))
        assertNull(disk.record)
        assertEquals(0, disk.saves)
        assertEquals(TermsStatus.Required(documents, changed = false), terms.status.value)
    }

    @Test fun `a failed write keeps the gate instead of claiming acceptance`() {
        val disk = Disk(writable = false)
        val terms = useCase(disk)
        assertFalse(terms.accept(ticked = true))
        assertEquals(TermsStatus.Required(documents, changed = false), terms.status.value)
    }

    @Test fun `a record for older documents returns to the gate as changed`() {
        val disk = Disk(TermsAcceptance("2026-09", "1.0", now - 1000))
        assertEquals(TermsStatus.Required(documents, changed = true), useCase(disk).status.value)
    }

    @Test fun `an unreadable store or a future record counts as no acceptance`() {
        assertEquals(TermsStatus.Required(documents, changed = false), useCase(Disk(readable = false)).status.value)
        val future = Disk(TermsAcceptance("2026-10", "1.0", now + 86_400_000L))
        assertEquals(TermsStatus.Required(documents, changed = false), useCase(future).status.value)
    }

    @Test fun `revoking deletes the record and returns to the gate at once`() {
        val disk = Disk()
        val terms = useCase(disk)
        terms.accept(ticked = true)
        assertTrue(terms.revoke())
        assertNull(disk.record)
        assertEquals(TermsStatus.Required(documents, changed = false), terms.status.value)
        assertEquals(TermsStatus.Required(documents, changed = false), useCase(disk).status.value)
    }

    @Test fun `a failed revocation keeps the accepted state that is still on disk`() {
        val disk = Disk()
        val terms = useCase(disk)
        terms.accept(ticked = true)
        disk.writable = false
        assertFalse(terms.revoke())
        assertTrue(terms.status.value is TermsStatus.Accepted)
    }
}
