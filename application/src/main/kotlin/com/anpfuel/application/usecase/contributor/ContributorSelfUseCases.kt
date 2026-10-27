package com.anpfuel.application.usecase.contributor

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.contributor.Claim
import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.IssuedClaim
import com.anpfuel.domain.contributor.ProfileEdit
import com.anpfuel.domain.contributor.ProfileRead
import com.anpfuel.domain.contributor.isValidGistId
import com.anpfuel.domain.contributor.isValidGithubLogin
import com.anpfuel.domain.contributor.profileEditError
import com.anpfuel.domain.repository.ContributorSelfGateway
import java.util.UUID

/**
 * B-BR-CTB contributor self-service (CTB-02/03/04/08), signed-in only.
 *
 * Every call loads the current session first; a missing session refuses
 * before any private call. Client-side input mirrors the server bounds;
 * the server stays authoritative and its codes surface verbatim.
 */
class OpenClaimUseCase(
    private val gateway: ContributorSelfGateway,
    private val sessions: AuthSessionStore,
) {
    suspend operator fun invoke(githubLogin: String): IssuedClaim {
        if (!isValidGithubLogin(githubLogin)) throw ContributorException.Invalid("contributors.invalid-login")
        val session = sessions.load() ?: throw ContributorException.Denied("contributors.signed-out")
        return gateway.openClaim(session.familyId, session.accessToken, githubLogin, UUID.randomUUID().toString())
    }
}

class ReadOwnClaimsUseCase(
    private val gateway: ContributorSelfGateway,
    private val sessions: AuthSessionStore,
) {
    suspend operator fun invoke(): List<Claim> {
        val session = sessions.load() ?: throw ContributorException.Denied("contributors.signed-out")
        return gateway.readClaims(session.familyId, session.accessToken)
    }
}

class SubmitClaimProofUseCase(
    private val gateway: ContributorSelfGateway,
    private val sessions: AuthSessionStore,
) {
    suspend operator fun invoke(claimId: String, gistId: String): Claim {
        if (!isValidGistId(gistId)) throw ContributorException.Invalid("contributors.invalid-gist")
        val session = sessions.load() ?: throw ContributorException.Denied("contributors.signed-out")
        return gateway.submitProof(session.familyId, session.accessToken, claimId, gistId)
    }
}

class WithdrawClaimUseCase(
    private val gateway: ContributorSelfGateway,
    private val sessions: AuthSessionStore,
) {
    suspend operator fun invoke(claimId: String): Claim {
        val session = sessions.load() ?: throw ContributorException.Denied("contributors.signed-out")
        return gateway.withdrawClaim(session.familyId, session.accessToken, claimId)
    }
}

class ReadOwnProfileUseCase(
    private val gateway: ContributorSelfGateway,
    private val sessions: AuthSessionStore,
) {
    suspend operator fun invoke(): ProfileRead {
        val session = sessions.load() ?: throw ContributorException.Denied("contributors.signed-out")
        return gateway.readProfile(session.familyId, session.accessToken)
    }
}

class UpsertProfileUseCase(
    private val gateway: ContributorSelfGateway,
    private val sessions: AuthSessionStore,
) {
    suspend operator fun invoke(edit: ProfileEdit): ProfileRead {
        profileEditError(edit)?.let { throw ContributorException.Invalid("contributors.invalid-$it") }
        val session = sessions.load() ?: throw ContributorException.Denied("contributors.signed-out")
        return gateway.upsertProfile(session.familyId, session.accessToken, edit)
    }
}
