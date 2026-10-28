package com.anpfuel.app.ui.profile

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.contributors.CONTRIBUTORS_ENTRY_TEST_TAG
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.application.portable.AuthAccountApi
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.portable.AuthKeyStore
import com.anpfuel.application.portable.AuthOperationLock
import com.anpfuel.application.portable.AuthPorts
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.portable.AuthWallClock
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.application.portable.PortableNonceSource
import com.anpfuel.app.ui.auth.AuthViewModel
import com.anpfuel.data.local.dao.PhotoUploadSessionDao
import com.anpfuel.data.local.entity.PhotoUploadSessionEntity
import com.anpfuel.data.repository.OwnerLocalErasure
import com.anpfuel.application.port.ContributionScopeProvider
import com.anpfuel.application.portable.PhotoCache
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.OwnedContribution
import com.anpfuel.domain.repository.QueuedContribution
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * B-BR-CTB contributors entry on Perfil.
 *
 * Renders the real [ProfileScreen] with a signed-out session store: no new
 * test dependency, no Hilt, no network. Hand fakes cover only the
 * construction seams; the order assertion runs against production UI code.
 */
@RunWith(AndroidJUnit4::class)
class ProfileContributorsCardTest {
    @get:Rule val compose = createComposeRule()

    private fun viewModel() = AuthViewModel(
        AuthFlow(
            AuthPorts(
                store = object : AuthSessionStore {
                    override fun save(session: PortableAuth.Session) = Unit
                    override fun load(): PortableAuth.Session? = null
                    override fun clear() = Unit
                },
                keys = object : AuthKeyStore {
                    override fun saveKey(backup: AuthFlow.KeyBackup) = Unit
                    override fun loadKey(): AuthFlow.KeyBackup? = null
                    override fun clearKey() = Unit
                },
                clock = AuthWallClock { 0L },
                nonces = PortableNonceSource { "nonce" },
                api = object : AuthAccountApi {
                    override fun requestCode(email: String) = TODO("unused")
                    override fun consumeCode(email: String, code: String) = TODO("unused")
                    override fun createKeyAccount(username: String) = TODO("unused")
                    override fun loginWithKey(accountKey: String) = TODO("unused")
                    override fun refresh(familyId: String, refreshToken: String) = TODO("unused")
                    override fun revokeAll(familyId: String, accessToken: String) = TODO("unused")
                    override fun linkProvider(
                        session: PortableAuth.Session,
                        provider: String,
                        idToken: String,
                        nonce: String,
                    ) = TODO("unused")

                    override fun deleteAccount(session: PortableAuth.Session) = TODO("unused")
                },
                lock = object : AuthOperationLock {
                    override fun <T> withLock(operation: () -> T): T = operation()
                },
            ),
        ),
        OwnerLocalErasure(
            outbox = object : ContributionOutboxRepository {
                override suspend fun markDispatched(commandId: String, nonce: String) = TODO("unused")
                override suspend fun markAcknowledged(commandId: String, revision: Int) = TODO("unused")
                override suspend fun markFailed(commandId: String, nowMillis: Long) = TODO("unused")
                override suspend fun cancel(commandId: String) = TODO("unused")
                override suspend fun listOwnedPage(limit: Int, offset: Int): List<OwnedContribution> = TODO("unused")
                override suspend fun findOwned(commandId: String): OwnedContribution? = TODO("unused")
                override suspend fun purgeTerminalHistory(): Int = TODO("unused")
                override suspend fun purgeOwned(ownerScope: String): Int = TODO("unused")
                override suspend fun enqueue(
                    draft: ContributionDraft,
                    payloadJson: String,
                ): QueuedContribution = TODO("unused")

                override suspend fun findDispatchable(commandId: String, nowMillis: Long): QueuedContribution? =
                    TODO("unused")

                override suspend fun listDispatchablePage(nowMillis: Long, limit: Int): List<QueuedContribution> =
                    TODO("unused")

                override suspend fun loadPayload(commandId: String): String? = TODO("unused")
            },
            photoSessions = object : PhotoUploadSessionDao {
                override suspend fun find(captureId: String): PhotoUploadSessionEntity? = null
                override suspend fun insert(session: PhotoUploadSessionEntity): Long = 0L
                override suspend fun ready(captureId: String, sessionId: String, evidenceId: String): Int = 0
                override suspend fun listByOwner(ownerScope: String): List<PhotoUploadSessionEntity> = emptyList()
                override suspend fun deleteByOwner(ownerScope: String): Int = 0
            },
            photoCache = object : PhotoCache {
                override fun put(id: String, bytes: ByteArray, capturedAtMillis: Long) = Unit
                override fun get(id: String): ByteArray? = null
                override fun delete(id: String) = Unit
                override fun sweepExpired(nowMillis: Long): Int = 0
            },
            scopeProvider = ContributionScopeProvider { TODO("unused") },
        ),
    )

    private fun render(opened: () -> Unit = {}) {
        compose.setContent {
            AnpFuelTheme(darkTheme = false) {
                ProfileScreen(
                    darkTheme = false,
                    onToggleTheme = {},
                    onNavigate = {},
                    onOpenPremium = {},
                    onOpenContributors = opened,
                    authViewModel = viewModel(),
                )
            }
        }
        compose.waitUntil(timeoutMillis = 10_000) {
            try {
                compose.onNodeWithTag(PROFILE_TOOL_PRIVACY_TEST_TAG).fetchSemanticsNode()
                true
            } catch (_: AssertionError) {
                false
            }
        }
    }

    @Test fun contributorsEntryIsDisplayedLastAfterPrivacy() {
        render()
        val entry = compose.onNodeWithTag(CONTRIBUTORS_ENTRY_TEST_TAG)
        entry.performScrollTo()
        entry.assertIsDisplayed()
        val privacyTop = compose.onNodeWithTag(PROFILE_TOOL_PRIVACY_TEST_TAG)
            .performScrollTo()
            .fetchSemanticsNode().boundsInRoot.top
        val entryTop = entry.fetchSemanticsNode().boundsInRoot.top
        assertTrue("contributors entry must be the last Profile item", entryTop > privacyTop)
    }

    @Test fun contributorsEntryClickOpensContributors() {
        var opened = false
        render(opened = { opened = true })
        compose.onNodeWithTag(CONTRIBUTORS_ENTRY_TEST_TAG)
            .performScrollTo()
            .performClick()
        assertTrue(opened)
    }
}
