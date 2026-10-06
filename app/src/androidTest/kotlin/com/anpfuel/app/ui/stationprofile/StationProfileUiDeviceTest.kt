package com.anpfuel.app.ui.stationprofile

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Density
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.application.usecase.profile.OwnedProfileClaim
import com.anpfuel.domain.profile.StationProfile
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Source UI proof only; real account/provider/TalkBack gestures are separate rows. */
@RunWith(AndroidJUnit4::class)
class StationProfileUiDeviceTest {
    @get:Rule val compose = createComposeRule()
    private fun label(id: Int) = InstrumentationRegistry.getInstrumentation().targetContext.getString(id)

    private fun capture(name: String) {
        val file = java.io.File(InstrumentationRegistry.getInstrumentation().targetContext.cacheDir, "$name.png")
        java.io.FileOutputStream(file).use { compose.onRoot().captureToImage().asAndroidBitmap().compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }
    }

    @Test fun stalePublicProfileHasReadableBadgeAndNoManagementAtDoubleFont() {
        compose.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, 2f)) {
                StationProfileContent(
                    StationProfileState(profile = StationProfile("station", "Public station", mapOf("phone" to "public phone"), hasBadge = true), stale = true),
                    {}, {},
                )
            }
        }
        compose.onNodeWithText(label(R.string.station_profile_stale)).assertIsDisplayed()
        capture("p32-public-font2")
        compose.onNodeWithText(label(R.string.station_profile_claim)).performScrollTo().assertIsNotEnabled()
        compose.onNodeWithText("private proof").assertDoesNotExist()
        compose.onNodeWithText(label(R.string.station_profile_verified)).assertDoesNotExist()
    }

    @Test fun signedPDFRequiresExplicitConfirmationAndGuestCannotOpenRequest() {
        var submissions = 0
        val claim = OwnedProfileClaim("c", "s", "manager", setOf("profile.edit"), "draft", "d", "synthetic declaration", 9999999999)
        compose.setContent {
            StationClaimContent(
                StationClaimState(selected = claim, fileBytes = 20), {}, {}, {}, {}, {}, {},
                onSubmit = { submissions++ }, onReissue = {}, onCancel = {}, onSignIn = {}, onManage = {},
            )
        }
        capture("p32-claim")
        assertEquals(0, submissions)
        compose.onNodeWithText(label(R.string.station_claim_send)).performScrollTo().performClick()
        assertEquals(1, submissions)
    }

    @Test fun guestStateOffersFreeLoginAndNoPrivateProofControls() {
        compose.setContent {
            StationClaimContent(StationClaimState(notice = ClaimNotice.SignIn), {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {})
        }
        compose.onNodeWithText(label(R.string.station_claim_login)).assertIsDisplayed()
        compose.onNodeWithText(label(R.string.station_claim_open)).assertDoesNotExist()
        compose.onNodeWithText(label(R.string.station_claim_pick)).assertDoesNotExist()
    }

    @Test fun approvedManagementFormRemainsScrollableAtDoubleFont() {
        compose.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, 2f)) {
                StationManagementContent(StationManagementState(fields = mapOf("phone" to "public phone"), revision = 1, scopes = setOf("profile.edit", "reply.official")), { _, _ -> }, {}, {}, {}, {}, {}, {})
            }
        }
        capture("p32-management-font2")
        compose.onNodeWithText(label(R.string.station_management_save)).performScrollTo().assertIsEnabled()
        compose.onNodeWithText(label(R.string.station_management_other_unavailable)).performScrollTo().assertIsDisplayed()
    }

    @Test fun revokedManagementHasNoEditableFieldsOrReplyControl() {
        compose.setContent { StationManagementContent(StationManagementState(notice = ManagementNotice.Refused), { _, _ -> }, {}, {}, {}, {}, {}, {}) }
        compose.onNodeWithText(label(R.string.station_claim_refused)).assertIsDisplayed()
        compose.onNodeWithText(label(R.string.station_management_save)).assertDoesNotExist()
        compose.onNodeWithText(label(R.string.station_management_publish)).assertDoesNotExist()
    }
}
