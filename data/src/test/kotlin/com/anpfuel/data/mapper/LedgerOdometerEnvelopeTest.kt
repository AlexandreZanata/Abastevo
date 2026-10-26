package com.anpfuel.data.mapper

import com.anpfuel.domain.premium.*
import java.time.Instant
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class LedgerOdometerEnvelopeTest {
    private val draft=ExpenseDraft("11111111-1111-4111-8111-111111111111","22222222-2222-4222-8222-222222222222",Instant.parse("2026-03-01T12:00:00Z"),"UTC",5000,ExpenseCategory.TOLLS,10000,null)
    @Test fun `legacy or unsupported proof is unverified while raw data remains readable`() {
        assertEquals(OdometerEvidence.UNVERIFIED,LedgerOdometerEnvelope.read(draft.bodyJson()).evidence)
        for (metadata in listOf(JSONObject().put("version",2).put("evidence","VERIFIED_METRES"),JSONObject().put("version",1).put("evidence","unknown"))) {
            val raw=JSONObject(draft.bodyJson()).put("_abastevo_odometer",metadata).toString()
            assertEquals(OdometerEvidence.UNVERIFIED,LedgerOdometerEnvelope.read(raw).evidence)
            assertEquals(draft,LedgerDraftCodec.decode(LedgerDataset.EXPENSE,raw))
        }
    }
    @Test fun `local proof roundtrips but never enters a command body`() {
        val body=LedgerOdometerEnvelope.write(draft,OdometerEvidence.VERIFIED_METRES)
        assertEquals(OdometerEvidence.VERIFIED_METRES,LedgerOdometerEnvelope.read(body).evidence)
        val decoded=LedgerDraftCodec.decode(LedgerDataset.EXPENSE,body)
        val command=LedgerCommandFactory.toCommand(PremiumOwnerScope("account","local-origin"),decoded,0)
        assertEquals(draft.bodyJson(),command.payloadJson)
        assertFalse(command.payloadJson.contains("_abastevo_odometer"))
    }
    @Test fun `receipt preserves numeric original without retaining old erased note`() {
        val old=draft.copy(note="old-private-note")
        val receipt=LedgerOdometerEnvelope.Receipt(10000,LegacyOdometerReview.fingerprint(old),0,"AS_KILOMETRES",LegacyOdometerReview.fingerprint(draft))
        val body=LedgerOdometerEnvelope.write(draft,OdometerEvidence.VERIFIED_METRES,receipt)
        assertEquals(receipt,LedgerOdometerEnvelope.read(body).receipt)
        assertFalse(body.contains("old-private-note"))
    }
}
