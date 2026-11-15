package com.anpfuel.domain.enterprise.review

/** Review dimension shown on a device; it never derives a server state from local work. */
data class ReviewPresentation(val state: ReviewState, val serverConfirmed: Boolean, val readyToSend: Boolean)

object ReviewPresenter {
    /**
     * Combines the last server-confirmed state (null when the server has no case) with an optional pending local
     * author command. Local work never advances the state; a local decision is not representable.
     */
    fun present(server: ReviewState?, local: ReviewCommandKind?): ReviewPresentation {
        if (local != null && local.role != ReviewRole.AUTHOR) throw reviewFailure(ReviewFailure.INVALID)
        if (server == null) {
            if (local != null && local != ReviewCommandKind.SUBMIT) throw reviewFailure(ReviewFailure.INVALID)
            return ReviewPresentation(ReviewState.DRAFT, serverConfirmed = false, readyToSend = local != null)
        }
        return ReviewPresentation(server, serverConfirmed = true, readyToSend = local != null)
    }
}
