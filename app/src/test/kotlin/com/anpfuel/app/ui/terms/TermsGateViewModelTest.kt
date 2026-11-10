package com.anpfuel.app.ui.terms

import com.anpfuel.application.port.TermsAcceptanceStore
import com.anpfuel.application.usecase.legal.TermsAcceptanceUseCase
import com.anpfuel.application.usecase.legal.TermsStatus
import com.anpfuel.domain.legal.LegalDocuments
import com.anpfuel.domain.legal.TermsAcceptance
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** B-BR-TA01..TA04 at the screen-state level; no Android framework involved. */
class TermsGateViewModelTest {
    private val documents = LegalDocuments("2026-10", "1.0")
    private val now = 1_800_000_000_000L

    private class Disk(var record: TermsAcceptance? = null, var writable: Boolean = true) : TermsAcceptanceStore {
        override fun load() = record
        override fun save(record: TermsAcceptance): Boolean { if (writable) this.record = record; return writable }
        override fun clear(): Boolean { if (writable) record = null; return writable }
    }

    private fun viewModel(disk: Disk) = TermsGateViewModel(TermsAcceptanceUseCase(disk, documents) { now })

    @Test fun `first open shows the gate with an unticked box`() {
        val vm = viewModel(Disk())
        assertEquals(TermsStatus.Required(documents, changed = false), vm.status.value)
        assertFalse(vm.ticked.value)
        assertFalse(vm.failed.value)
    }

    @Test fun `accept without a tick does nothing`() {
        val disk = Disk()
        val vm = viewModel(disk)
        vm.accept()
        assertNull(disk.record)
        assertTrue(vm.status.value is TermsStatus.Required)
        assertFalse(vm.failed.value)
    }

    @Test fun `tick then accept records versions and instant and opens the app`() {
        val disk = Disk()
        val vm = viewModel(disk)
        vm.tick(true)
        vm.accept()
        assertEquals(TermsAcceptance("2026-10", "1.0", now), disk.record)
        assertEquals(TermsStatus.Accepted(TermsAcceptance("2026-10", "1.0", now)), vm.status.value)
        assertFalse(vm.ticked.value)
    }

    @Test fun `unticking before accept records nothing`() {
        val disk = Disk()
        val vm = viewModel(disk)
        vm.tick(true)
        vm.tick(false)
        vm.accept()
        assertNull(disk.record)
        assertTrue(vm.status.value is TermsStatus.Required)
    }

    @Test fun `a failed write keeps the gate, reports it and drops the tick`() {
        val disk = Disk(writable = false)
        val vm = viewModel(disk)
        vm.tick(true)
        vm.accept()
        assertTrue(vm.status.value is TermsStatus.Required)
        assertTrue(vm.failed.value)
        assertFalse(vm.ticked.value)
        vm.tick(true)
        assertFalse(vm.failed.value)
    }

    @Test fun `revoking returns to an unticked gate and a second view model sees it at once`() {
        val disk = Disk()
        val terms = TermsAcceptanceUseCase(disk, documents) { now }
        val gate = TermsGateViewModel(terms)
        val privacy = TermsGateViewModel(terms)
        gate.tick(true)
        gate.accept()
        assertTrue(privacy.status.value is TermsStatus.Accepted)
        assertTrue(privacy.revoke())
        assertNull(disk.record)
        assertEquals(TermsStatus.Required(documents, changed = false), gate.status.value)
        assertFalse(gate.ticked.value)
    }

    @Test fun `a failed revocation keeps the app open and reports failure`() {
        val disk = Disk()
        val vm = viewModel(disk)
        vm.tick(true)
        vm.accept()
        disk.writable = false
        assertFalse(vm.revoke())
        assertTrue(vm.status.value is TermsStatus.Accepted)
    }

    @Test fun `a record for older documents shows the gate as changed`() {
        val vm = viewModel(Disk(TermsAcceptance("2026-09", "1.0", now - 1)))
        assertEquals(TermsStatus.Required(documents, changed = true), vm.status.value)
    }

    @Test fun `stored instant is shown as a local date and time`() {
        val previous = java.util.Locale.getDefault()
        try {
            java.util.Locale.setDefault(java.util.Locale.forLanguageTag("pt-BR"))
            val text = acceptedAtText(1_794_314_260_000L, java.time.ZoneId.of("America/Cuiaba"))
            assertTrue(text.contains("2026"), text)
        } finally { java.util.Locale.setDefault(previous) }
    }
}
