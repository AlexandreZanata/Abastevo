package com.anpfuel.domain.repository

import com.anpfuel.domain.contributor.Claim
import com.anpfuel.domain.contributor.IssuedClaim
import com.anpfuel.domain.contributor.ProfileEdit
import com.anpfuel.domain.contributor.ProfileRead

/**
 * B-BR-CTB signed-in contributor self-service (CTB-02/03/04/08).
 * Every call carries the owner family/access proof; the server enforces
 * quotas, states and capabilities.
 */
interface ContributorSelfGateway {
    suspend fun openClaim(familyId: String, accessToken: String, githubLogin: String, operationId: String): IssuedClaim
    suspend fun readClaims(familyId: String, accessToken: String): List<Claim>
    suspend fun submitProof(familyId: String, accessToken: String, claimId: String, gistId: String): Claim
    suspend fun withdrawClaim(familyId: String, accessToken: String, claimId: String): Claim
    suspend fun readProfile(familyId: String, accessToken: String): ProfileRead
    suspend fun upsertProfile(familyId: String, accessToken: String, edit: ProfileEdit): ProfileRead
}
