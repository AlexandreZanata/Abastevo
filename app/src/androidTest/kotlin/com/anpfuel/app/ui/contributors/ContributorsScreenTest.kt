package com.anpfuel.app.ui.contributors

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.contributor.Contributor
import com.anpfuel.domain.contributor.ContributorLink
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** B-BR-CTB public list content states (no ViewModel, no network). */
@RunWith(AndroidJUnit4::class)
class ContributorsScreenTest {
    @get:Rule val compose = createComposeRule()

    private val ana = Contributor(
        displayName = "Ana",
        links = listOf(ContributorLink("github", "octocat", "https://github.com/octocat")),
        githubLogin = "octocat",
        contributorSince = "2026-03",
        roles = listOf("Suporte"),
    )

    private fun render(
        state: ContributorsUiState,
        onOpenLink: (String) -> Unit = {},
        onRetry: () -> Unit = {},
    ) {
        compose.setContent {
            AnpFuelTheme(darkTheme = false) {
                ContributorsContent(
                    state = state,
                    onNavigateBack = {},
                    onClaimClick = {},
                    onRetry = onRetry,
                    onLoadMore = {},
                    onOpenLink = onOpenLink,
                )
            }
        }
    }

    @Test fun contributorRowShowsNameBadgeRolesAndLinks() {
        var opened: String? = null
        render(ContributorsUiState(loading = false, items = listOf(ana)), onOpenLink = { opened = it })
        compose.onNodeWithText("Ana").assertIsDisplayed()
        compose.onNodeWithText("Suporte").assertIsDisplayed()
        compose.onNodeWithText("octocat").performClick()
        assertEquals("https://github.com/octocat", opened)
    }

    @Test fun emptyStateHasNoRows() {
        render(ContributorsUiState(loading = false, items = emptyList()))
        compose.onNodeWithText("Ana").assertDoesNotExist()
    }

    @Test fun errorStateRetries() {
        var retried = false
        render(ContributorsUiState(loading = false, notice = ContributorsNotice.ERROR), onRetry = { retried = true })
        compose.onNodeWithText("Retry").performClick()
        assertEquals(true, retried)
    }

    @Test fun sameNameAndMonthDoNotCollide() {
        render(ContributorsUiState(loading = false, items = listOf(ana, ana.copy(githubLogin = null))))
        assertEquals(2, compose.onAllNodesWithText("Ana").fetchSemanticsNodes().size)
    }
}
