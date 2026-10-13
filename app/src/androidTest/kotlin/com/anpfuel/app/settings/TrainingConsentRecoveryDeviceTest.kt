package com.anpfuel.app.settings

import android.content.Context
import android.content.ContextWrapper
import android.content.SharedPreferences
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.NetworkType
import androidx.work.WorkInfo
import androidx.work.WorkManager
import com.anpfuel.application.port.ContributionScopeProvider
import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.application.usecase.privacy.ResumeTrainingRevocation
import com.anpfuel.application.usecase.privacy.TrainingConsentCaller
import com.anpfuel.application.usecase.privacy.TrainingConsentOutcome
import com.anpfuel.application.usecase.privacy.TrainingConsentUseCase
import com.anpfuel.data.local.preferences.TrainingConsentStateStore
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.repository.*
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import java.util.UUID
import java.util.concurrent.TimeUnit

/** Real Android disk, synthetic owners/transport: never sends a real user's privacy decision. */
@RunWith(AndroidJUnit4::class)
class TrainingConsentRecoveryDeviceTest {
    /** Read-only observation of the real Application/Hilt scheduler on the connected device. */
    @Test fun applicationSchedulesConnectedRecoveryAndStartupWorkerCompletes() {
        val manager=WorkManager.getInstance(ApplicationProvider.getApplicationContext())
        val periodic=manager.getWorkInfosForUniqueWork("training-revocation-periodic").get(10,TimeUnit.SECONDS)
        assertEquals(1,periodic.size)
        assertFalse(periodic.single().state.isFinished)
        assertEquals(NetworkType.CONNECTED,periodic.single().constraints.requiredNetworkType)
        assertEquals(TimeUnit.MINUTES.toMillis(15),periodic.single().periodicityInfo!!.repeatIntervalMillis)
        val startup=manager.getWorkInfosForUniqueWork("training-revocation-startup").get(10,TimeUnit.SECONDS)
        assertEquals(1,startup.size)
        assertEquals(NetworkType.CONNECTED,startup.single().constraints.requiredNetworkType)
        assertEquals(WorkInfo.State.SUCCEEDED,startup.single().state)
    }

    @Test fun offlineOffReopensAndRetriesOnlyForOriginalOwnerAfterPersistedDeadline() = runBlocking {
        val context=ApplicationProvider.getApplicationContext<Context>()
        val name="consent-recovery-test-"+UUID.randomUUID()
        val isolated=object:ContextWrapper(context) {
            override fun getSharedPreferences(requested:String,mode:Int):SharedPreferences =
                context.getSharedPreferences(name,mode)
        }
        val a=ContributionScope("account:synthetic-a:key","https://test.invalid")
        val b=a.copy(ownerScope="account:synthetic-b:key")
        val caller=TrainingConsentCaller("synthetic-a",a)
        var current=a
        var online=false
        var now=1000L
        var reads=0
        val sends=mutableListOf<Pair<String,Long>>()
        val gateway=object:TrainingConsentGateway {
            override suspend fun consentStatus(scope:ContributionScope):TrainingConsentStatus {
                assertEquals(a,scope);reads++
                return TrainingConsentStatus(false,null,null,null,100,false)
            }
            override suspend fun grantConsent(scope:ContributionScope,termsVersion:String,termsDigest:String,revision:Long,commandId:String):TrainingConsentReceipt =
                error("Recovery must never grant permission")
            override suspend fun revokeConsent(scope:ContributionScope,revision:Long,commandId:String):TrainingConsentReceipt {
                assertEquals(a,scope);sends.add(commandId to revision)
                if(sends.size==1) throw TrainingConsentException(TrainingConsentRejectKind.TRANSPORT,"retry",120)
                return TrainingConsentReceipt("REVOKED",revision,"",true,true)
            }
        }
        val network=object:NetworkConnectivityGateway {override fun observeIsConnected()=flowOf(online)}
        val scopes=ContributionScopeProvider {current}
        fun use(local:TrainingConsentStateStore)=TrainingConsentUseCase(local,gateway,network,scopes,{now},{"native-off"})
        try {
            val first=TrainingConsentStateStore(isolated)
            assertEquals(TrainingConsentOutcome.RevocationPending,use(first).revoke(caller))
            assertEquals(0,reads);assertTrue(sends.isEmpty())
            val original=first.load(a).pending!!
            var reopened=TrainingConsentStateStore(isolated)
            current=b
            ResumeTrainingRevocation(reopened,use(reopened))(TrainingConsentCaller("synthetic-b",b))
            val foreign=use(reopened).syncPending(caller) as TrainingConsentOutcome.Rejected
            assertEquals(TrainingConsentRejectKind.AUTH_REQUIRED,foreign.kind)
            assertEquals(original,reopened.load(a).pending)
            assertEquals(0,reads);assertTrue(sends.isEmpty())
            current=a
            ResumeTrainingRevocation(reopened,use(reopened))(caller)
            assertEquals(0,reads);assertTrue(sends.isEmpty())
            online=true
            ResumeTrainingRevocation(reopened,use(reopened))(caller)
            val failed=reopened.load(a)
            assertEquals(121000L,failed.retryAtMillis)
            assertEquals(1,failed.attempts)
            assertEquals(listOf("native-off" to 101L),sends)
            reopened=TrainingConsentStateStore(isolated)
            now=120999L
            ResumeTrainingRevocation(reopened,use(reopened))(caller)
            assertEquals(1,reads);assertEquals(1,sends.size)
            assertEquals(failed,reopened.load(a))
            now=121000L
            ResumeTrainingRevocation(reopened,use(reopened))(caller)
            assertEquals(listOf("native-off" to 101L,"native-off" to 101L),sends)
            val acknowledged=TrainingConsentStateStore(isolated).load(a)
            assertNull(acknowledged.pending);assertFalse(acknowledged.granted)
            assertEquals(101L,acknowledged.revisionFloor)
            assertEquals(0L,acknowledged.retryAtMillis)
            ResumeTrainingRevocation(reopened,use(reopened))(caller)
            assertEquals(1,reads);assertEquals(2,sends.size)
        } finally {context.deleteSharedPreferences(name)}
    }
}
