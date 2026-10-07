package com.anpfuel.application.usecase.contribution

import com.anpfuel.application.port.ContributionOutboxFlagProvider
import com.anpfuel.application.port.ContributionScopeProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.model.PhotoContributionContext
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.QueuedContribution
import com.anpfuel.domain.repository.PendingContribution
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

sealed interface EnqueueReviewOutcome {
    data object Disabled : EnqueueReviewOutcome
    data class Queued(val commands: List<QueuedContribution>, val historical: Boolean) : EnqueueReviewOutcome
}

class EnqueueContributionUseCase(
    private val flagProvider: ContributionOutboxFlagProvider,
    private val outbox: ContributionOutboxRepository,
    private val nowMillis: () -> Long = { System.currentTimeMillis() },
    private val scopeProvider: ContributionScopeProvider? = null,
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
        val photoContext: PhotoContributionContext? = null,
    )

    suspend fun invoke(request: Request): EnqueueContributionOutcome {
        if (!flagProvider.isEnabled()) {
            return EnqueueContributionOutcome.Disabled
        }
        val draft = validate(request)
        val now = nowMillis()
        val historical = ContributionStalenessRule.isHistorical(draft.capturedAtMillis, now)
        val scope = resolveScope(request.photoContext)
        val payload = encodePayload(draft, historical, scope, request.photoContext)
        val queued = outbox.enqueue(draft, payload)
        return EnqueueContributionOutcome.Queued(queued, historical)
    }

    private fun validate(request: Request): ContributionDraft {
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
        return draft
    }

    private suspend fun resolveScope(context: PhotoContributionContext?): ContributionScope? {
        val current = scopeProvider?.currentScope()
        if (context != null && current != null && context.scope != current) {
            throw DomainException("photo owner or environment changed")
        }
        return current ?: context?.scope
    }

    suspend fun invokeReview(requests: List<Request>): EnqueueReviewOutcome {
        if (!flagProvider.isEnabled()) return EnqueueReviewOutcome.Disabled
        if (requests.isEmpty() || requests.size > FuelProduct.entries.size ||
            requests.map { it.clientSubmissionId }.distinct().size != requests.size ||
            requests.map { it.fuelProduct }.distinct().size != requests.size) {
            throw DomainException("review must contain distinct fuel rows")
        }
        val first = requests.first()
        val context = first.photoContext
        val now = nowMillis()
        if (first.photoId == null || context == null || now >= context.expiresAtMillis ||
            first.capturedAtMillis > now || now - first.capturedAtMillis >= 86_400_000L ||
            requests.any { it.stationId != first.stationId || it.photoId != first.photoId ||
                it.photoContext != context || it.capturedAtMillis != first.capturedAtMillis || it.conditionKind != "STANDARD" }) {
            throw DomainException("photo review expired or inconsistent")
        }
        val drafts = requests.map(::validate)
        val scope = resolveScope(context)
        val historical = ContributionStalenessRule.isHistorical(first.capturedAtMillis, now)
        val commands = drafts.map { PendingContribution(it, encodePayload(it, historical, scope, context)) }
        return EnqueueReviewOutcome.Queued(outbox.enqueueReview(commands), historical)
    }

    internal fun encodePayload(draft: ContributionDraft, historical: Boolean, scope: ContributionScope? = null, context: PhotoContributionContext? = null): String {
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
        val scopeJson = scope?.let { "{\"owner_scope\":\"${esc(it.ownerScope)}\",\"origin\":\"${esc(it.origin)}\"}" } ?: "null"
        val captureJson = context?.let { "{\"capture_id\":\"${it.captureId}\",\"expires_at_millis\":${it.expiresAtMillis}}" } ?: "null"
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
            "\"supersedes_observation_id\":$supersedes,\"scope\":$scopeJson,\"photo_capture\":$captureJson}"
    }
}
