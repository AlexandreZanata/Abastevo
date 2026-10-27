package com.anpfuel.app.ui.contributors

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.contributor.ProfileEdit
import com.anpfuel.domain.contributor.ProfileRead
import com.anpfuel.domain.contributor.ProfileVisibility
import com.anpfuel.domain.contributor.StoredProfile
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** B-BR-CTB profile form content (no ViewModel, no network). */
@RunWith(AndroidJUnit4::class)
class ProfileEditScreenTest {
    @get:Rule val compose = createComposeRule()

    private fun render(
        state: ProfileUiState,
        onSave: (ProfileEdit) -> Unit = {},
    ) {
        compose.setContent {
            AnpFuelTheme(darkTheme = false) {
                ProfileContent(
                    state = state,
                    onSave = onSave,
                    onRetry = {},
                    onNavigateBack = {},
                )
            }
        }
    }

    @Test fun existingProfilePrefillsName() {
        render(
            ProfileUiState(
                loading = false,
                read = ProfileRead(
                    StoredProfile("Ana", emptyList(), ProfileVisibility.PUBLIC, true, false),
                    roles = listOf("Suporte"),
                ),
            ),
        )
        compose.onNodeWithText("Ana").assertIsDisplayed()
        compose.onNodeWithText("Suporte").assertIsDisplayed()
    }

    @Test fun publicWithoutConsentNeverSaves() {
        var saved: ProfileEdit? = null
        render(ProfileUiState(loading = false, read = ProfileRead(null, emptyList())), onSave = { saved = it })
        compose.onNodeWithTag(PROFILE_NAME_FIELD_TAG).performTextInput("Ana")
        compose.onNodeWithTag(PROFILE_VISIBILITY_SWITCH_TAG).performClick()
        compose.onNodeWithTag(PROFILE_SAVE_BUTTON_TAG).performClick()
        assertNull(saved)
    }

    @Test fun consentedSaveCapturesEdit() {
        var saved: ProfileEdit? = null
        render(ProfileUiState(loading = false, read = ProfileRead(null, emptyList())), onSave = { saved = it })
        compose.onNodeWithTag(PROFILE_NAME_FIELD_TAG).performTextInput("Ana")
        compose.onNodeWithTag(PROFILE_VISIBILITY_SWITCH_TAG).performClick()
        compose.onNodeWithTag(PROFILE_CONSENT_CHECK_TAG).performClick()
        compose.onNodeWithTag(PROFILE_SAVE_BUTTON_TAG).performClick()
        val edit = saved ?: throw AssertionError("expected save")
        assertEquals("Ana", edit.displayName)
        assertEquals(ProfileVisibility.PUBLIC, edit.visibility)
        assertEquals(true, edit.publicConsent)
    }
}
