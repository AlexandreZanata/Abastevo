package com.anpfuel.app.settings

import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.application.portable.*
import com.anpfuel.data.local.auth.AndroidAnonymousDeviceKeys
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.domain.portable.PortableAuth
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import java.util.UUID

/** Only two generated test-owner aliases; existing keys are never removed or adopted. */
@RunWith(AndroidJUnit4::class)
class TrainingConsentKeyStoreDeviceTest {
 @Test fun realKeystoreSeparatesAccountsAndRetainsOriginalOwnerAfterLogout() {
  val base="consent-device-test-"+UUID.randomUUID()
  val a=PortableAuth.Session("family",base+"-a","access","refresh",1000,2000)
  val b=a.copy(accountId=base+"-b")
  var session:PortableAuth.Session?=a
  val flow=AuthFlow(AuthPorts(object:AuthSessionStore {
   override fun load()=session
   override fun save(value:PortableAuth.Session) {session=value}
   override fun clear() {session=null}
  },object:AuthKeyStore {
   override fun loadKey():AuthFlow.KeyBackup?=null
   override fun saveKey(backup:AuthFlow.KeyBackup)=error("unexpected backup")
   override fun clearKey()=Unit
  },AuthWallClock {100},PortableNonceSource {"test"},object:AuthAccountApi {
   override fun requestCode(email:String):AuthApiResult<Unit> = error("unexpected network")
   override fun consumeCode(email:String,code:String):AuthApiResult<ConsumeOk> = error("unexpected network")
   override fun createKeyAccount(username:String):AuthApiResult<AuthFlow.KeyIssued> = error("unexpected network")
   override fun loginWithKey(accountKey:String):AuthApiResult<KeyLogin> = error("unexpected network")
   override fun refresh(familyId:String,refreshToken:String):AuthApiResult<PortableAuth.Session> = error("unexpected network")
   override fun revokeAll(familyId:String,accessToken:String):AuthApiResult<Unit> = error("unexpected network")
   override fun linkProvider(session:PortableAuth.Session,provider:String,idToken:String,nonce:String):AuthApiResult<Unit> = error("unexpected network")
   override fun deleteAccount(session:PortableAuth.Session):AuthApiResult<Unit> = error("unexpected network")
  }))
  val keys=AndroidAnonymousDeviceKeys(flow,ApiEnvironment.STAGING)
  try {
   assertTrue(keys.ensureKey());val first=keys.fingerprint();assertNotNull(first)
   session=b;assertTrue(keys.ensureKey());assertNotEquals(first,keys.fingerprint())
   session=null;assertNotEquals(first,keys.fingerprint()) // Does not create/remove the anonymous key.
   session=a;assertEquals(first,keys.fingerprint());assertNotNull(keys.signDigest(ByteArray(64){1}))
  } finally {
   session=a;keys.forgetKey();session=b;keys.forgetKey()
  }
 }
}
