package com.anpfuel.data.local.preferences

import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.repository.TrainingConsentException
import java.util.concurrent.Executors
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class TrainingConsentStateStoreTest {
    private val a=ContributionScope("account:a:key","https://test.invalid")
    private class Disk {
        var raw:String?=null
        var accept=true
        fun open()=TrainingConsentStateStore({raw},{ raw=it;accept })
    }
    @Test fun `OFF and exact retry survive reconstruction without leaking owner identifiers`() {
        val disk=Disk();val store=disk.open()
        val state=store.update(a) { it.revoke("off").bindRevocation("off",100).let { b -> b.failed(b.pending!!,1000,3600) } }
        assertEquals(state,disk.open().load(a))
        assertFalse(disk.raw!!.contains("account:a:key"));assertFalse(disk.raw!!.contains("https://test.invalid"))
        for (scope in listOf(a.copy(ownerScope="account:b:key"),a.copy(ownerScope="account:a:other"),a.copy(origin="https://other.invalid"))) {
            assertNull(disk.open().load(scope).pending)
        }
    }
    @Test fun `atomic scoped updates never lose concurrent revocations`() {
        val disk=Disk();val store=disk.open();val pool=Executors.newFixedThreadPool(4)
        try { (1..24).map { i -> pool.submit { store.update(a.copy(ownerScope="account:$i:key")) { it.revoke("off-$i") } } }.forEach { it.get() } }
        finally { pool.shutdownNow() }
        for(i in 1..24) assertEquals("off-$i",disk.open().load(a.copy(ownerScope="account:$i:key")).pending!!.id)
    }
    @Test fun `failed commit poisons RAM view and refuses authority`() {
        val disk=Disk();val store=disk.open();disk.accept=false
        assertThrows(TrainingConsentException::class.java) { store.update(a) { it.revoke("off") } }
        assertThrows(TrainingConsentException::class.java) { store.load(a) }
    }
    @Test fun `corrupt pending and oversize journal refuse instead of discarding OFF`() {
        val disk=Disk();disk.open().update(a) { it.revoke("off") }
        val doc=JSONObject(disk.raw!!);doc.getJSONObject(doc.keys().next()).put("pending","corrupt");disk.raw=doc.toString()
        assertThrows(TrainingConsentException::class.java) { disk.open().load(a) }
        disk.raw=" ".repeat(65537)
        assertThrows(TrainingConsentException::class.java) { disk.open().load(a) }
    }
    @Test fun `full journal refuses new owners without evicting pending rights`() {
        val disk=Disk();val store=disk.open()
        repeat(32) { i -> store.update(a.copy(ownerScope="account:$i:key")) { it.revoke("off-$i") } }
        assertThrows(TrainingConsentException::class.java) { store.update(a) { it.revoke("new") } }
        assertEquals("off-0",disk.open().load(a.copy(ownerScope="account:0:key")).pending!!.id)
    }
}
