package com.anpfuel.app.ui.contributors

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.contributor.Claim
import com.anpfuel.domain.contributor.ClaimProof
import com.anpfuel.domain.contributor.ClaimState
import com.anpfuel.domain.contributor.IssuedClaim
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** B-BR-CTB claim flow content (no ViewModel, no network). */
@RunWith(AndroidJUnit4::class)
class ClaimScreenTest {
    @get:Rule val compose = createComposeRule()

    private val pending = Claim(
        id = "c1", githubLogin = "octocat", state = ClaimState.PENDING, proof = ClaimProof.UNVERIFIED,
        gistId = null, override = false, createdAt = "", expiresAt = "",
    )

    private fun render(
        state: ClaimUiState,
        onOpenClaim: (String) -> Unit = {},
        onSubmitProof: (String, String) -> Unit = { _, _ -> },
        onWithdraw: (String) -> Unit = {},
        onCopyCode: (String) -> Unit = {},
    ) {
        compose.setContent {
            AnpFuelTheme(darkTheme = false) {
                ClaimContent(
                    state = state,
                    onOpenClaim = onOpenClaim,
                    onSubmitProof = onSubmitProof,
                    onWithdraw = onWithdraw,
                    onDismissIssued = {},
                    onCopyCode = onCopyCode,
                    onEditProfile = {},
                    onRetry = {},
                    onNavigateBack = {},
                )
            }
        }
    }

    @Test fun issuedCodeShowsCopyButton() {
        var copied: String? = null
        render(
            ClaimUiState(loading = false, issued = IssuedClaim("c1", "abastevo-contributor-AAAAAAAAAAAAAAAA")),
            onCopyCode = { copied = it },
        )
        compose.onNodeWithText("abastevo-contributor-AAAAAAAAAAAAAAAA").assertIsDisplayed()
        compose.onNodeWithTag(CLAIM_COPY_BUTTON_TAG).performClick()
        assertEquals("abastevo-contributor-AAAAAAAAAAAAAAAA", copied)
    }

    @Test fun openClaimCapturesLogin() {
        var login: String? = null
        render(ClaimUiState(loading = false), onOpenClaim = { login = it })
        compose.onNodeWithTag(CLAIM_LOGIN_FIELD_TAG).performTextInput("octocat")
        compose.onNodeWithTag(CLAIM_OPEN_BUTTON_TAG).performClick()
        assertEquals("octocat", login)
    }

    @Test fun pendingClaimSubmitsProofAndWithdraws() {
        val proofs = mutableListOf<Pair<String, String>>()
        val withdrawn = mutableListOf<String>()
        render(
            ClaimUiState(loading = false, claims = listOf(pending)),
            onSubmitProof = { id, gist -> proofs.add(id to gist) },
            onWithdraw = { withdrawn.add(it) },
        )
        compose.onNodeWithText("octocat").assertIsDisplayed()
        compose.onNodeWithTag(CLAIM_GIST_FIELD_TAG).performTextInput("gist9")
        compose.onNodeWithTag(CLAIM_PROVE_BUTTON_TAG).performClick()
        assertEquals(listOf("c1" to "gist9"), proofs)
        compose.onNodeWithTag(CLAIM_WITHDRAW_BUTTON_TAG).performClick()
        assertEquals(listOf("c1"), withdrawn)
    }

    @Test fun decidedClaimShowsStateWithoutProofForm() {
        render(ClaimUiState(loading = false, claims = listOf(pending.copy(state = ClaimState.APPROVED))))
        compose.onNodeWithText("Approved").assertIsDisplayed()
        compose.onNodeWithTag(CLAIM_GIST_FIELD_TAG).assertDoesNotExist()
    }

    @Test fun pendingClaimRedisplaysItsCode() {
        var copied: String? = null
        render(
            ClaimUiState(loading = false, claims = listOf(pending.copy(code = "abastevo-contributor-CCCCCCCCCCCCCCCC"))),
            onCopyCode = { copied = it },
        )
        compose.onNodeWithText("abastevo-contributor-CCCCCCCCCCCCCCCC").assertIsDisplayed()
        compose.onNodeWithTag(CLAIM_ROW_COPY_BUTTON_TAG).performClick()
        assertEquals("abastevo-contributor-CCCCCCCCCCCCCCCC", copied)
    }
}
