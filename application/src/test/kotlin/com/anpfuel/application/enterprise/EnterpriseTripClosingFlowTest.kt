package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.expense.*
import com.anpfuel.application.enterprise.fueling.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.trip.*
import com.anpfuel.domain.enterprise.expense.ExpenseOperations
import com.anpfuel.domain.enterprise.fueling.*
import com.anpfuel.domain.enterprise.trip.*
import java.time.ZoneId
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/**
 * EMP-TRIP-020 (B-BR-EMP-522..526, EDR-339..341): the trip owner's closing conference over the real trip, fueling and
 * expense services of one protected partition, and the first submission it offers.
 */
class EnterpriseTripClosingFlowTest {
    private class Fixture {
        val trips = TripFlowFixture()
        // One queue for the partition's trip, fueling and expense records, as on the device.
        val all = EnterpriseQueueService(trips.source, trips.queue, { _, _ -> throw EnterpriseQueueDeliveryFailure() },
            EnterpriseQueueRegistry(*EnterpriseTripProducers.all(trips.hints), EnterpriseFuelingProducer(), EnterpriseExpenseProducer()), trips.clock, { 0 })
        var fuelingsFail = false
        val fuelings = EnterpriseFuelingService(trips.source, all::save, EnterpriseFuelingStore { attempt ->
            if (fuelingsFail) throw EnterprisePersistenceUnavailable()
            trips.disk.of(attempt.partition).entries.values.filter { it.operation.type == FuelingOperations.TYPE }
        }, { ZoneId.of("America/Cuiaba") }, { trips.wall })
        val expenses = EnterpriseExpenseService(trips.source, all::save, EnterpriseExpenseStore { attempt ->
            trips.disk.of(attempt.partition).entries.values.filter { it.operation.type == ExpenseOperations.SUBMIT }
        }, { ZoneId.of("America/Cuiaba") }, { trips.wall })
        val service = EnterpriseTripService(trips.source, all::save, trips.store, trips.hints, trips.clock, { ZoneId.of("America/Cuiaba") }, { trips.wall })
        fun flow() = TripFlow(trips.source, service, trips.fence, null, TripRelatedRecords(fuelings::fuelings, expenses::expenses)) { _, _ -> }
        fun fueling(org: EnterpriseContextSummary = trips.a, asset: Int = 70, paid: String = "970,00") = FuelingForm(FuelProduct.DIESEL_S10, "150", paid,
            null, null, FillLevel.FULL, Payer.COMPANY, PaymentMethod.FUEL_CARD, TripCapture.Absent("Painel apagado"))
    }

    @Test fun `a finished trip confers this vehicle's fuelings and expenses of the trip and offers its first submission`() = runTest {
        val x = Fixture()
        val flow = x.flow()
        flow.open(x.trips.a, x.trips.fence.epoch)
        flow.start(x.trips.choice(), x.trips.label, x.trips.km("184320"), x.trips.fence.epoch, flow.state.value.generation)
        val tripId = flow.state.value.trips.single().trip.tripId
        x.trips.later(30)
        val account = x.trips.source.account()
        x.fuelings.record(account, x.trips.a, x.trips.choice(), x.fueling())
        // Distinct instants: items are listed by occurrence (equal instants fall back to random record identifiers).
        x.trips.later(1)
        x.expenses.record(account, x.trips.a, x.trips.choice(), ExpenseForm("TOLL", "12,50", "12,50", ExpensePayer.EMPLOYEE))
        // Another vehicle and another organization are never this trip's.
        x.fuelings.record(account, x.trips.a, x.trips.choice(asset = 71), x.fueling())
        x.expenses.record(account, x.trips.b, x.trips.choice(org = x.trips.b), ExpenseForm("PARKING", "8", "8", ExpensePayer.COMPANY))
        x.trips.later(30)
        flow.finish(tripId, x.trips.km("184610"), x.trips.fence.epoch, flow.state.value.generation)
        x.trips.later(10)
        // After the finish nothing more belongs to this trip.
        x.expenses.record(account, x.trips.a, x.trips.choice(), ExpenseForm("PARKING", "8", "8", ExpensePayer.COMPANY))
        flow.reload(x.trips.fence.epoch, flow.state.value.generation)
        val closing = requireNotNull(flow.state.value.closing(tripId))
        assertEquals(listOf(ClosingItemKind.DEPARTURE_READING, ClosingItemKind.FUELING, ClosingItemKind.EXPENSE, ClosingItemKind.FINAL_READING),
            closing.items.map { it.kind })
        assertEquals(listOf(97_000L, 1_250L), closing.items.mapNotNull { it.paidCentavos })
        assertTrue(closing.items.all { it.record == ClosingRecordState.SAVED_ON_DEVICE && it.evidence == ClosingEvidenceState.MISSING })
        assertEquals(ClosingSubmission.READY_TO_MARK, closing.submission)
        assertEquals(1L, closing.revision)
        // The submission it offers is the TRIP-005 first submission: revision 1, depending on the finish.
        flow.markReady(tripId, x.trips.fence.epoch, flow.state.value.generation)
        val view = flow.state.value.trips.single()
        val submit = view.submission!!.fact as TripOperationFact.Submit
        assertEquals(1L, submit.revision)
        assertEquals(listOf(view.trip.finishOperationId), submit.dependsOn)
        assertEquals(ClosingSubmission.ALREADY_MARKED, flow.state.value.closing(tripId)!!.submission)
        assertNull(flow.state.value.closing(tripId)!!.revision)
        // Marking again queues nothing (ALREADY_SUBMITTED), never a second submission.
        val count = x.trips.entries().size
        flow.markReady(tripId, x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(TripLocalFailure.ALREADY_SUBMITTED, flow.state.value.refusal)
        assertEquals(count, x.trips.entries().size)
    }

    @Test fun `unreadable related records are said and never shown as nothing to confer`() = runTest {
        val x = Fixture()
        val flow = x.flow()
        flow.open(x.trips.a, x.trips.fence.epoch)
        flow.start(x.trips.choice(), x.trips.label, x.trips.km("184320"), x.trips.fence.epoch, flow.state.value.generation)
        val tripId = flow.state.value.trips.single().trip.tripId
        x.fuelingsFail = true
        flow.reload(x.trips.fence.epoch, flow.state.value.generation)
        val closing = requireNotNull(flow.state.value.closing(tripId))
        assertTrue(closing.relatedUnavailable)
        assertNull(flow.state.value.error, "An unreadable related list never hides the trips")
        assertEquals(ClosingSubmission.NOT_FINISHED, closing.submission)
        // Without conference sources there is no conference.
        val plain = x.trips.flow()
        plain.open(x.trips.a, x.trips.fence.epoch)
        assertNull(plain.state.value.closing(tripId))
    }
}
