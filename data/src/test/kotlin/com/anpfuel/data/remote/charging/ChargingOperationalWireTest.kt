package com.anpfuel.data.remote.charging

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.charging.*
import com.anpfuel.domain.charging.*
import com.anpfuel.domain.portable.PortableAuth
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assumptions.assumeTrue
import org.junit.jupiter.api.Assertions.*
import java.util.UUID
import java.time.Instant

/** Actual Go HTTP process + disposable PostGIS; not a MockWebServer proof. */
class ChargingOperationalWireTest {
    @Test fun `submit replay edit history and withdraw traverse Go PostGIS and durable Kotlin outbox`() = runBlocking {
        val origin=System.getenv("ANPFUEL_CHARGING_WIRE_ORIGIN")
        assumeTrue(!origin.isNullOrBlank(), "opt-in Go process required")
        val client=ChargingObservationHttpClient(OkHttpClient(), origin!!)
        val family="bbbbbbbb-1111-4111-8111-000000000001"
        val token="synthetic-wire-proof"
        val session=PortableAuth.Session(family,"aaaaaaaa-1111-4111-8111-000000000001",token,"unused",Long.MAX_VALUE,Long.MAX_VALUE)
        val auth=object:AuthSessionStore { override fun load()=session;override fun save(session:PortableAuth.Session)=Unit;override fun clear()=Unit }
        val prefs=mutableMapOf<String,String>()
        val store=object:ChargingPendingStore {
            override fun load(accountId:String,origin:String)=prefs[accountId]?.let(com.anpfuel.data.local.preferences.ChargingPendingCodec::decode).orEmpty()
            override fun save(accountId:String,origin:String,operations:List<PendingChargingOperation>) {prefs[accountId]=com.anpfuel.data.local.preferences.ChargingPendingCodec.encode(operations)}
            override fun clear(accountId:String,origin:String){prefs.remove(accountId)}
        }
        val place="bbbbbbbb-1111-4111-8111-000000000001"
        val draft=ChargingObservationDraft(place,ChargingConnector.TYPE2,6790,ChargingCondition.STANDARD,
            java.time.LocalDate.now().toString(),"Synthetic wire",ChargingOperationalEvent(ChargingOperationalStatus.WORKED,ChargingTargetScope.SITE,null,Instant.now().minusSeconds(60).truncatedTo(java.time.temporal.ChronoUnit.MICROS).toString()))
        val uc=ChargingCommunityUseCase(client,store,auth,origin)
        assertTrue(uc.save(draft) is ChargingSaveOutcome.Saved)
        val owned=client.mine(family,token,20).single()
        assertEquals(draft.operational,owned.operational)
        val changed=draft.copy(note="Synthetic corrected",operational=draft.operational!!.copy(event=ChargingOperationalStatus.DID_NOT_WORK))
        val operation=UUID.randomUUID().toString()
        repeat(2){assertEquals(2L,client.correctOperation(family,token,owned.id,1,changed,operation))}
        val history=client.history(place,20).single()
        assertEquals(ChargingOperationalStatus.DID_NOT_WORK,history.operational!!.event)
        val withdraw=UUID.randomUUID().toString()
        repeat(2){assertEquals(3L,client.withdrawOperation(family,token,owned.id,2,withdraw))}
        assertTrue(client.history(place,20).isEmpty())
    }
}
