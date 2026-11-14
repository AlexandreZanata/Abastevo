package com.anpfuel.app.ui.enterprise

import android.app.KeyguardManager
import android.app.Instrumentation
import android.content.Context
import android.content.Intent
import android.os.SystemClock
import android.security.keystore.UserNotAuthenticatedException
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.enterprise.AndroidEnterpriseVaultKeys
import com.anpfuel.data.enterprise.EnterpriseKeyUse
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Explicit synthetic managed policy, independent of the still-gated server
 * managed policy. Only the task-owned emulator/isolated package may run it.
 * A real Android credential prompt must authorize actual Cipher operations.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseManagedKeyDeviceTest {
    @Test
    fun realCredentialAuthorizesOnlyTheConfiguredWindowAndDeletedHandleFails() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val context = instrumentation.targetContext
        check(context.packageName == "com.anpfuel.app.enterprisevalidation")
        val pin = InstrumentationRegistry.getArguments().getString("synthetic_pin")
        require(pin != null && Regex("[0-9]{6}").matches(pin)) { "Supply this task emulator's synthetic credential" }
        val keyguard = context.getSystemService(KeyguardManager::class.java)
        assertTrue("Set a credential only on the task-owned emulator", keyguard.isDeviceSecure)
        val keys = AndroidEnterpriseVaultKeys()
        // A scoped synthetic V1-shaped alias exercises the same provider policy
        // used for protected V2 payload keys, without altering any real lease.
        val alias = "abastevo_enterprise_v1_" + "1".repeat(64) + "-" + "2".repeat(64)
        try {
            assertNull(keys.existing(alias))
            val key = keys.create(alias, SYNTHETIC_WINDOW_SECONDS).secret
            assertNull(key.encoded)
            assertEquals(SYNTHETIC_WINDOW_SECONDS, keys.existing(alias)!!.managedUnlockSeconds)
            // Do not mistake the recent adb lock-credential setup for this
            // explicit ceremony: wait for any previous short window to expire.
            waitUntilUnauthenticated(key, keys)
            val sealed = authorize(instrumentation, context, keyguard, pin, key)
            val decrypt = Cipher.getInstance("AES/GCM/NoPadding")
            decrypt.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, sealed.first))
            assertArrayEquals(SYNTHETIC_PAYLOAD, decrypt.doFinal(sealed.second))
            assertNotNull(keys.existing(alias)) // The alias survives expiration.
            waitUntilUnauthenticated(key, keys)
            // Reauthorize before deletion. Otherwise an authentication refusal
            // (also InvalidKeyException) could falsely pass the deletion test.
            authorize(instrumentation, context, keyguard, pin, key)
            keys.delete(alias)
            assertNull(keys.existing(alias))
            try { seal(key); fail("A deleted Android Keystore handle remained usable") }
            catch (_: UserNotAuthenticatedException) { fail("Deletion was tested outside the authorization window") }
            catch (_: java.security.InvalidKeyException) { /* Actual Keystore rejects old opaque handle. */ }
            assertFalse(KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.containsAlias(alias))
        } finally { keys.delete(alias) }
    }

    private fun authorize(instrumentation: Instrumentation, context: Context, keyguard: KeyguardManager,
        pin: String, key: SecretKey): Pair<ByteArray, ByteArray> {
        @Suppress("DEPRECATION")
        val prompt = keyguard.createConfirmDeviceCredentialIntent(PROMPT_TITLE, "Unlock the disposable emulator key")
            ?: error("Credential prompt unavailable")
        val expectedPackage = prompt.resolveActivity(context.packageManager)?.packageName ?: error("Credential activity unavailable")
        check(expectedPackage in setOf("com.android.settings", "com.android.systemui"))
        instrumentation.uiAutomation.serviceInfo = instrumentation.uiAutomation.serviceInfo.apply {
            flags = flags or android.accessibilityservice.AccessibilityServiceInfo.FLAG_REPORT_VIEW_IDS
        }
        context.startActivity(prompt.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        val deadline = SystemClock.elapsedRealtime() + 15000
        var field: android.view.accessibility.AccessibilityNodeInfo? = null
        while (SystemClock.elapsedRealtime() < deadline && field == null) {
            val root = instrumentation.uiAutomation.rootInActiveWindow
            if (root != null && isCredentialWindow(root, expectedPackage)) field = credentialField(root)
            if (field == null) SystemClock.sleep(100)
        }
        val input = field ?: error("The resolved Android credential field did not appear")
        val credentialPackage = input.packageName?.toString()
        check(credentialPackage in setOf(expectedPackage, "com.android.systemui") && input.isFocused && input.refresh())
        check(instrumentation.uiAutomation.rootInActiveWindow?.let { isCredentialWindow(it, expectedPackage) } == true)
        // Target the exact system credential node, never globally type into
        // whichever unrelated window may become active during the test.
        val text = android.os.Bundle().apply {
            putCharSequence(android.view.accessibility.AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, pin)
        }
        check(input.performAction(android.view.accessibility.AccessibilityNodeInfo.ACTION_SET_TEXT, text)) {
            "Credential node refused scoped text entry"
        }
        check(input.refresh() && input.packageName?.toString() == credentialPackage && input.isFocused)
        check(instrumentation.uiAutomation.rootInActiveWindow?.let { isCredentialWindow(it, expectedPackage) } == true)
        check(input.performAction(android.view.accessibility.AccessibilityNodeInfo.AccessibilityAction.ACTION_IME_ENTER.id)) {
            "Credential node refused scoped submission"
        }
        val authorizationDeadline = SystemClock.elapsedRealtime() + 10000
        while (SystemClock.elapsedRealtime() < authorizationDeadline) {
            try { return seal(key) }
            catch (_: UserNotAuthenticatedException) { SystemClock.sleep(100) }
        }
        error("The credential did not authorize actual key use")
    }

    private fun isCredentialWindow(root: android.view.accessibility.AccessibilityNodeInfo, resolvedPackage: String): Boolean {
        val actual = root.packageName?.toString()
        if (actual == "com.android.systemui") {
            // API36 Settings delegates its real credential ceremony to SystemUI.
            // Inspect that exact system header/title/input instead of accepting
            // an arbitrary password field in another system window.
            return root.findAccessibilityNodeInfosByViewId("com.android.systemui:id/auth_credential_header").isNotEmpty() &&
                root.findAccessibilityNodeInfosByViewId("com.android.systemui:id/title").any { it.text?.toString() == PROMPT_TITLE } &&
                root.findAccessibilityNodeInfosByViewId("com.android.systemui:id/lockPassword").isNotEmpty()
        }
        return actual == resolvedPackage && root.findAccessibilityNodeInfosByText(PROMPT_TITLE).any { it.text?.toString() == PROMPT_TITLE }
    }

    private fun credentialField(node: android.view.accessibility.AccessibilityNodeInfo): android.view.accessibility.AccessibilityNodeInfo? {
        if (node.isEditable && node.isPassword && node.isFocused) return node
        for (index in 0 until node.childCount) node.getChild(index)?.let(::credentialField)?.let { return it }
        return null
    }

    private fun waitUntilUnauthenticated(key: SecretKey, keys: AndroidEnterpriseVaultKeys) {
        val deadline = SystemClock.elapsedRealtime() + (SYNTHETIC_WINDOW_SECONDS + 8) * 1000
        while (SystemClock.elapsedRealtime() < deadline) {
            try { seal(key) }
            catch (failure: UserNotAuthenticatedException) {
                assertEquals(EnterpriseKeyUse.UNLOCK_REQUIRED, keys.classify(failure))
                return
            }
            SystemClock.sleep(100)
        }
        fail("Managed key remained authorized beyond its configured Keystore window")
    }

    private fun seal(key: SecretKey): Pair<ByteArray, ByteArray> = Cipher.getInstance("AES/GCM/NoPadding").run {
        init(Cipher.ENCRYPT_MODE, key)
        iv to doFinal(SYNTHETIC_PAYLOAD)
    }

    companion object {
        private const val SYNTHETIC_WINDOW_SECONDS = 5
        private const val PROMPT_TITLE = "Synthetic enterprise acceptance"
        private val SYNTHETIC_PAYLOAD = "SYNTHETIC-MANAGED-KEY-WINDOW".toByteArray()
    }
}
