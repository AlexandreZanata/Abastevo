package com.anpfuel.data.local.auth
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.domain.portable.PortableAuth
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
class AnonymousKeyScopeTest {
 @Test fun `account key aliases isolate server owners and preserve expired owner recovery`() {
  val session=PortableAuth.Session("family","account-a","access","refresh",100,200)
  val a=AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(session),ApiEnvironment.STAGING.origin)
  assertEquals(a,AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.NeedsRefresh(session),ApiEnvironment.STAGING.origin))
  assertNotEquals(a,AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(session.copy(accountId="account-b")),ApiEnvironment.STAGING.origin))
  assertNotEquals(a,AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.Active(session),"https://other.invalid"))
  assertFalse(a.contains("account-a"));assertTrue(a.startsWith(AndroidAnonymousDeviceKeys.ACCOUNT_ALIAS_PREFIX))
  assertEquals(AndroidAnonymousDeviceKeys.DEFAULT_ALIAS,AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.LoggedOut,ApiEnvironment.STAGING.origin))
  assertNotEquals(a,AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.LoggedOut,ApiEnvironment.STAGING.origin))
  assertNotEquals(AndroidAnonymousDeviceKeys.DEFAULT_ALIAS,AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.LoggedOut,"https://other.invalid"))
  assertThrows(IllegalStateException::class.java) {AndroidAnonymousDeviceKeys.aliasFor(AuthFlow.AuthState.PendingKeyAccount(AuthFlow.KeyBackup("user","key")),ApiEnvironment.STAGING.origin)}
 }
}
