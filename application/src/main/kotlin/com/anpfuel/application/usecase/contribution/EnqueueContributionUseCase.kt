package com.anpfuel.application.usecase.contribution

import com.anpfuel.application.port.ContributionOutboxFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.QueuedContribution
import com.anpfuel.domain.rule.ContributionStalenessRule
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P10-T05 — Durable contribution enqueue (BUC-003, B-BR-003/005/011).
 *
 * Flag disabled: [Outcome.Disabled], caller keeps browsing/metadata-only
 * paths and nothing is persisted. Otherwise the draft is validated
 * (station/product/milli/condition), labelled fresh/historical by capture
 * age (old captures stay historical, never promoted to "now") and stored
 * durably under its stable client submission id: a repeated id bumps the
 * revision in place (attempts kept) so duplicate send/process death yields
 * one observation, never two. The payload carries no contributor id, GPS,
 * EXIF or signed URL (B-BR-011). Dispatch (fresh nonce per send,
 * reserve/PUT/complete) belongs to the WorkManager worker, never here.
 */
sealed interface EnqueueContributionOutcome {
    data object Disabled : EnqueueContributionOutcome
    data class Queued(
        val command: QueuedContribution,
        val historical: Boolean,
    ) : EnqueueContributionOutcome
}

class EnqueueContributionUseCase(
    private val flagProvider: ContributionOutboxFlagProvider,
    private val outbox: ContributionOutboxRepository,
    private val nowMillis: () -> Long = { System.currentTimeMillis() },
) {
    data class Request(
        val clientSubmissionId: String,
        val stationId: String,
        val fuelProduct: FuelProduct,
        val amountMilliBrl: Long,
        val conditionKind: String,
        val capturedAtMillis: Long,
        val photoId: String? = null,
        val supersedesObservationId: String? = null,
        val unit: String = "BRL/L",
        val currency: String = "BRL",
    )

    suspend fun invoke(request: Request): EnqueueContributionOutcome {
        if (!flagProvider.isEnabled()) {
            return EnqueueContributionOutcome.Disabled
        }
        if (request.clientSubmissionId.isBlank()) {
            throw DomainException("client_submission_id is blank")
        }
        val draft = ContributionDraft.create(
            clientSubmissionId = request.clientSubmissionId,
            stationId = request.stationId,
            fuelProduct = request.fuelProduct,
            amountMilliBrl = request.amountMilliBrl,
            currency = request.currency,
            unit = request.unit,
            conditionKind = request.conditionKind,
            capturedAtMillis = request.capturedAtMillis,
            photoId = request.photoId,
            supersedesObservationId = request.supersedesObservationId,
        )
        val now = nowMillis()
        val historical = ContributionStalenessRule.isHistorical(draft.capturedAtMillis, now)
        val payload = encodePayload(draft, historical)
        val queued = outbox.enqueue(draft, payload)
        return EnqueueContributionOutcome.Queued(queued, historical)
    }

    internal fun encodePayload(draft: ContributionDraft, historical: Boolean): String {
        fun esc(value: String): String = value
            .replace("\\", "\\\\")
            .replace("\"", "\\\"")
        val photoId = draft.photoId
        val photo = if (photoId == null) "null" else "\"${esc(photoId)}\""
        val supersedesId = draft.supersedesObservationId
        val supersedes = if (supersedesId == null) {
            "null"
        } else {
            "\"${esc(supersedesId)}\""
        }
        val freshness = if (historical) "historical" else "fresh"
        return "{\"client_submission_id\":\"${esc(draft.clientSubmissionId)}\"," +
            "\"station_id\":\"${esc(draft.stationId)}\"," +
            "\"fuel_product\":\"${draft.fuelProduct.name}\"," +
            "\"amount_milli_brl\":${draft.amountMilliBrl}," +
            "\"currency\":\"${esc(draft.currency)}\"," +
            "\"unit\":\"${esc(draft.unit)}\"," +
            "\"condition_kind\":\"${esc(draft.conditionKind)}\"," +
            "\"captured_at_millis\":${draft.capturedAtMillis}," +
            "\"freshness\":\"$freshness\"," +
            "\"photo_id\":$photo," +
            "\"supersedes_observation_id\":$supersedes}"
    }
}
