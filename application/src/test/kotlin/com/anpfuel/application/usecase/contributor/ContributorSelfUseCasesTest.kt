package com.anpfuel.application.usecase.contributor

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.contributor.Claim
import com.anpfuel.domain.contributor.ClaimProof
import com.anpfuel.domain.contributor.ClaimState
import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.IssuedClaim
import com.anpfuel.domain.contributor.ProfileEdit
import com.anpfuel.domain.contributor.ProfileRead
import com.anpfuel.domain.contributor.ProfileVisibility
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.repository.ContributorSelfGateway
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ContributorSelfUseCasesTest {

    private val session = PortableAuth.Session("fam", "acct", "token", "refresh", Long.MAX_VALUE, Long.MAX_VALUE)
    private val sessions = object : AuthSessionStore {
        var current: PortableAuth.Session? = session
        override fun save(session: PortableAuth.Session) = Unit
        override fun load(): PortableAuth.Session? = current
        override fun clear() = Unit
    }
    private val claim = Claim(
        id = "c1", githubLogin = "octocat", state = ClaimState.PENDING, proof = ClaimProof.UNVERIFIED,
        gistId = null, override = false, createdAt = "", expiresAt = "",
    )
    private val calls = mutableListOf<String>()
    private val gateway = object : ContributorSelfGateway {
        override suspend fun openClaim(familyId: String, accessToken: String, githubLogin: String, operationId: String): IssuedClaim {
            calls.add("open:$familyId:$accessToken:$githubLogin")
            return IssuedClaim("c1", "abastevo-contributor-AAAAAAAAAAAAAAAA")
        }

        override suspend fun readClaims(familyId: String, accessToken: String): List<Claim> {
            calls.add("read")
            return listOf(claim)
        }

        override suspend fun submitProof(familyId: String, accessToken: String, claimId: String, gistId: String): Claim {
            calls.add("proof:$claimId:$gistId")
            return claim
        }

        override suspend fun withdrawClaim(familyId: String, accessToken: String, claimId: String): Claim {
            calls.add("withdraw:$claimId")
            return claim
        }

        override suspend fun readProfile(familyId: String, accessToken: String): ProfileRead {
            calls.add("profile-read")
            return ProfileRead(null, emptyList())
        }

        override suspend fun upsertProfile(familyId: String, accessToken: String, edit: ProfileEdit): ProfileRead {
            calls.add("profile-save:${edit.displayName}")
            return ProfileRead(null, emptyList())
        }
    }

    @Test fun `signed out refuses before any private call`() = runTest {
        sessions.current = null
        try {
            OpenClaimUseCase(gateway, sessions)("octocat")
            throw AssertionError("expected Denied")
        } catch (e: ContributorException.Denied) {
            assertEquals("contributors.signed-out", e.message)
        }
        assertTrue(calls.isEmpty())
        sessions.current = session
    }

    @Test fun `claim flow carries the session proof`() = runTest {
        assertEquals("c1", OpenClaimUseCase(gateway, sessions)("octocat").claimId)
        assertEquals(1, ReadOwnClaimsUseCase(gateway, sessions)().size)
        SubmitClaimProofUseCase(gateway, sessions)("c1", "gist1")
        WithdrawClaimUseCase(gateway, sessions)("c1")
        assertTrue(calls.contains("open:fam:token:octocat"))
        assertTrue(calls.contains("proof:c1:gist1"))
        assertTrue(calls.contains("withdraw:c1"))
    }

    @Test fun `profile flow carries the session proof`() = runTest {
        ReadOwnProfileUseCase(gateway, sessions)()
        UpsertProfileUseCase(gateway, sessions)(
            ProfileEdit("Ana", emptyList(), ProfileVisibility.HIDDEN, false, false),
        )
        assertTrue(calls.contains("profile-read"))
        assertTrue(calls.contains("profile-save:Ana"))
    }

    @Test fun `invalid input refuses before any private call`() = runTest {
        for (block in listOf<suspend () -> Unit>(
            { OpenClaimUseCase(gateway, sessions)("-bad-") },
            { SubmitClaimProofUseCase(gateway, sessions)("c1", "has space") },
            {
                UpsertProfileUseCase(gateway, sessions)(
                    ProfileEdit("Ana", emptyList(), ProfileVisibility.PUBLIC, false, false),
                )
            },
        )) {
            try {
                block()
                throw AssertionError("expected Invalid")
            } catch (_: ContributorException.Invalid) {
            }
        }
        assertTrue(calls.isEmpty())
    }
}
