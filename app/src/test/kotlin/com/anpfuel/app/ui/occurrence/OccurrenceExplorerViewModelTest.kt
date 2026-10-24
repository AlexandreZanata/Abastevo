package com.anpfuel.app.ui.occurrence

import com.anpfuel.domain.occurrence.*
import com.anpfuel.domain.repository.OccurrenceGateway
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class OccurrenceExplorerViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @BeforeEach
    fun before() {
        Dispatchers.setMain(dispatcher)
    }

    @AfterEach
    fun after() {
        Dispatchers.resetMain()
    }

    private val record = OccurrenceRecord("eeeeeeee-6666-4666-8666-000000000002", "enforcement-source:abc",
        "company-shaped", "DF", "BRASILIA", "Revenda", "2026-10-04", "document_day",
        "712508", "Inspection", "Resultado", "synthetic.xlsx#row:2")
    private val lease = OccurrenceLease(null, true, false)
    private fun gateway(
        items: List<OccurrenceRecord> = listOf(record),
        next: String? = null,
        stale: Boolean = false,
        fail: Boolean = false,
    ) = object : OccurrenceGateway {
        override suspend fun records(uf: String?, municipality: String?, segment: String?, subjectKind: String?, fromDay: String?, toDay: String?, limit: Int, cursor: String?, revisionId: String?): OccurrencePage<OccurrenceRecord> {
            if (fail) throw java.io.IOException("down")
            return OccurrencePage(items, next, lease, stale)
        }
        override suspend fun recordCoverage(revisionId: String?): Pair<RecordCoverage, OccurrenceLease> =
            RecordCoverage(items.size.toLong(), 1, "2026-10-04", "2026-10-04", "document_day") to lease
        override suspend fun stations(state: String?, segment: String?, fromDay: String?, toDay: String?, limit: Int, cursor: String?, revisionId: String?): OccurrencePage<OccurrenceStation> =
            OccurrencePage(emptyList(), null, lease)
        override suspend fun stationCoverage(revisionId: String?): Pair<StationCoverage, OccurrenceLease> =
            StationCoverage(0, 0) to lease
        override suspend fun record(id: String, revisionId: String?): Pair<OccurrenceRecord, OccurrenceLease> =
            record to lease
    }

    @Test fun `loads first page with coverage and preserves state`() = runTest(dispatcher) {
        val viewModel = OccurrenceExplorerViewModel(fakeSavedState(), gateway())
        advanceUntilIdle()
        val state = viewModel.state.value
        assertEquals(1, state.items.size)
        assertEquals(1L, state.coverage?.observations)
        assertFalse(state.loading)
        assertFalse(state.unavailable)
    }

    @Test fun `filter change resets list and reloads`() = runTest(dispatcher) {
        val viewModel = OccurrenceExplorerViewModel(fakeSavedState(), gateway())
        advanceUntilIdle()
        viewModel.selectFilter(OccurrenceFilter(uf = "SP", segment = "Revenda de GLP", subjectKind = "company-shaped"))
        advanceUntilIdle()
        assertEquals("SP", viewModel.state.value.filter.uf)
        assertEquals("Revenda de GLP", viewModel.state.value.filter.segment)
        assertEquals(1, viewModel.state.value.items.size)
        viewModel.clearFilters()
        advanceUntilIdle()
        assertTrue(viewModel.state.value.filter.isEmpty())
    }

    @Test fun `repeated refresh does not restart an active equivalent read`() = runTest(dispatcher) {
        val gate = kotlinx.coroutines.CompletableDeferred<Unit>()
        var calls = 0
        val slow = object : OccurrenceGateway by gateway() {
            override suspend fun records(uf: String?, municipality: String?, segment: String?, subjectKind: String?, fromDay: String?, toDay: String?, limit: Int, cursor: String?, revisionId: String?): OccurrencePage<OccurrenceRecord> {
                calls++
                gate.await()
                return OccurrencePage(listOf(record), null, lease, false)
            }
        }
        val viewModel = OccurrenceExplorerViewModel(fakeSavedState(), slow)
        runCurrent()
        viewModel.refresh()
        runCurrent()
        viewModel.refresh()
        runCurrent()
        assertEquals(1, calls)
        gate.complete(Unit)
        advanceUntilIdle()
        assertEquals(1, viewModel.state.value.items.size)
    }

    @Test fun `timeline separates company and source-global records`() = runTest(dispatcher) {
        val company = record
        val global = record.copy(id = "eeeeeeee-6666-4666-8666-000000000003", subjectKind = "personal-shaped", documentDay = "2019-01-02")
        val sections = timelineSections(listOf(global, company))
        assertEquals(2, sections.size)
        assertTrue(sections[0].companyKind)
        assertEquals(listOf(company.id), sections[0].items.map { it.id })
        assertEquals(listOf(global.id), sections[1].items.map { it.id })
    }

    @Test fun `failure with empty list marks unavailable without crashing`() = runTest(dispatcher) {
        val viewModel = OccurrenceExplorerViewModel(fakeSavedState(), gateway(fail = true))
        advanceUntilIdle()
        assertTrue(viewModel.state.value.unavailable)
        assertTrue(viewModel.state.value.items.isEmpty())
    }

    @Test fun `stale gateway surfaces stale flag`() = runTest(dispatcher) {
        val viewModel = OccurrenceExplorerViewModel(fakeSavedState(), gateway(stale = true))
        advanceUntilIdle()
        assertTrue(viewModel.state.value.stale)
    }

    private fun fakeSavedState(): androidx.lifecycle.SavedStateHandle = androidx.lifecycle.SavedStateHandle()
}
