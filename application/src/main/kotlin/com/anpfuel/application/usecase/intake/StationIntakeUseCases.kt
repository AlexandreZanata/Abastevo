package com.anpfuel.application.usecase.intake

import com.anpfuel.domain.repository.StationIntakeGateway
import java.io.IOException

/**
 * P27-T04 suggestion/status transport use cases.
 *
 * Session tokens travel in every call body (never URLs); the stable
 * client submission id makes offline retries replay instead of
 * duplicating the visible station. HTTP failures map to explicit
 * outcomes — a transport failure is `Unavailable`, never an invented
 * acceptance. Account proof stays session-based; no new trust is
 * granted here.
 */
sealed interface SubmitSuggestionOutcome {
    data class Submitted(val id: String) : SubmitSuggestionOutcome
    data object Conflict : SubmitSuggestionOutcome
    data object QuotaExceeded : SubmitSuggestionOutcome
    data object SessionInvalid : SubmitSuggestionOutcome
    data object Invalid : SubmitSuggestionOutcome
    data class Unavailable(val cause: Exception) : SubmitSuggestionOutcome
}

sealed interface SuggestionStatusOutcome {
    data class Found(val id: String, val state: String) : SuggestionStatusOutcome
    data object NotFound : SuggestionStatusOutcome
    data object SessionInvalid : SuggestionStatusOutcome
    data class Unavailable(val cause: Exception) : SuggestionStatusOutcome
}

sealed interface CancelSuggestionOutcome {
    data object Cancelled : CancelSuggestionOutcome
    data object Closed : CancelSuggestionOutcome
    data object NotFound : CancelSuggestionOutcome
    data object SessionInvalid : CancelSuggestionOutcome
    data class Unavailable(val cause: Exception) : CancelSuggestionOutcome
}

sealed interface OwnedSuggestionsOutcome {
    data class Found(val items: List<com.anpfuel.domain.repository.IntakeSummary>) : OwnedSuggestionsOutcome
    data object SessionInvalid : OwnedSuggestionsOutcome
    data class Unavailable(val cause: Exception) : OwnedSuggestionsOutcome
}

class SubmitStationSuggestionUseCase(
    private val gateway: StationIntakeGateway,
) {
    fun invoke(
        familyId: String,
        accessToken: String,
        clientSubmissionId: String,
        proposalJson: String,
    ): SubmitSuggestionOutcome {
        if (familyId.isBlank() || accessToken.isBlank()) {
            return SubmitSuggestionOutcome.SessionInvalid
        }
        if (clientSubmissionId.isBlank() || proposalJson.isBlank()) {
            return SubmitSuggestionOutcome.Invalid
        }
        return try {
            val receipt = gateway.submit(familyId, accessToken, clientSubmissionId, proposalJson)
            SubmitSuggestionOutcome.Submitted(receipt.id)
        } catch (error: IOException) {
            val message = error.message.orEmpty()
            when {
                message.contains("HTTP 401") -> SubmitSuggestionOutcome.SessionInvalid
                message.contains("HTTP 409") -> SubmitSuggestionOutcome.Conflict
                message.contains("HTTP 429") -> SubmitSuggestionOutcome.QuotaExceeded
                message.contains("HTTP 400") -> SubmitSuggestionOutcome.Invalid
                else -> SubmitSuggestionOutcome.Unavailable(error)
            }
        }
    }
}

class GetSuggestionStatusUseCase(
    private val gateway: StationIntakeGateway,
) {
    fun invoke(familyId: String, accessToken: String, id: String): SuggestionStatusOutcome {
        if (familyId.isBlank() || accessToken.isBlank()) {
            return SuggestionStatusOutcome.SessionInvalid
        }
        if (id.isBlank()) return SuggestionStatusOutcome.NotFound
        return try {
            val status = gateway.status(familyId, accessToken, id)
            SuggestionStatusOutcome.Found(id = status.id, state = status.state)
        } catch (error: IOException) {
            val message = error.message.orEmpty()
            when {
                message.contains("HTTP 401") -> SuggestionStatusOutcome.SessionInvalid
                message.contains("HTTP 404") -> SuggestionStatusOutcome.NotFound
                else -> SuggestionStatusOutcome.Unavailable(error)
            }
        }
    }
}

class CancelOwnedSuggestionUseCase(
    private val gateway: StationIntakeGateway,
) {    fun invoke(familyId: String, accessToken: String, id: String): CancelSuggestionOutcome {
        if (familyId.isBlank() || accessToken.isBlank()) {
            return CancelSuggestionOutcome.SessionInvalid
        }
        if (id.isBlank()) return CancelSuggestionOutcome.NotFound
        return try {
            gateway.cancel(familyId, accessToken, id)
            CancelSuggestionOutcome.Cancelled
        } catch (error: IOException) {
            val message = error.message.orEmpty()
            when {
                message.contains("HTTP 401") -> CancelSuggestionOutcome.SessionInvalid
                message.contains("HTTP 404") -> CancelSuggestionOutcome.NotFound
                message.contains("HTTP 409") -> CancelSuggestionOutcome.Closed
                else -> CancelSuggestionOutcome.Unavailable(error)
            }
        }
    }
}

class GetOwnedSuggestionsUseCase(
    private val gateway: StationIntakeGateway,
) {
    fun invoke(familyId: String, accessToken: String): OwnedSuggestionsOutcome {
        if (familyId.isBlank() || accessToken.isBlank()) {
            return OwnedSuggestionsOutcome.SessionInvalid
        }
        return try {
            OwnedSuggestionsOutcome.Found(gateway.mine(familyId, accessToken))
        } catch (error: IOException) {
            val message = error.message.orEmpty()
            if (message.contains("HTTP 401")) {
                OwnedSuggestionsOutcome.SessionInvalid
            } else {
                OwnedSuggestionsOutcome.Unavailable(error)
            }
        }
    }
}
