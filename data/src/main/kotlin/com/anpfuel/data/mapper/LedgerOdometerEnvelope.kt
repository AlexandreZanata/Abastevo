package com.anpfuel.data.mapper

import com.anpfuel.domain.premium.*
import org.json.JSONObject

/** Private Room envelope only. Outgoing commands reconstruct LedgerDraft.bodyJson(). */
internal object LedgerOdometerEnvelope {
    private const val KEY = "_abastevo_odometer"
    data class Receipt(val original: Long, val base: String, val revision: Long, val choice: String, val result: String)
    data class Proof(val evidence: OdometerEvidence, val receipt: Receipt? = null)

    fun read(body: String): Proof = try {
        val meta = JSONObject(body).optJSONObject(KEY)
        if (meta == null || meta.optInt("version", -1) != 1) Proof(OdometerEvidence.UNVERIFIED)
        else {
            val evidence = OdometerEvidence.valueOf(meta.getString("evidence"))
            val rec = meta.optJSONObject("review")?.let {
                Receipt(it.getLong("original"), it.getString("base"), it.getLong("revision"), it.getString("choice"), it.getString("result"))
            }
            Proof(evidence, rec)
        }
    } catch (_: Exception) { Proof(OdometerEvidence.UNVERIFIED) }

    fun write(draft: LedgerDraft, evidence: OdometerEvidence, receipt: Receipt? = null): String {
        val meta = JSONObject().put("version", 1).put("evidence", evidence.name)
        receipt?.let { meta.put("review", JSONObject().put("original", it.original).put("base", it.base)
            .put("revision",it.revision).put("choice",it.choice).put("result",it.result)) }
        return JSONObject(draft.bodyJson()).put(KEY, meta).toString()
    }
}
