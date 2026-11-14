package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class EnterpriseVaultContactBindingTest {
    @TempDir lateinit var root: Path
    @Test fun changedMembershipRequiresReviewedEmptyResetAndNeverRetagsHeldIntent(): Unit = runBlocking {
        val f = VaultFixture(root); val p = partition()
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        val contacts = f.contacts
        assertThrows(EnterpriseVaultUnrecoverable::class.java) { runBlocking { f.confirm(p, membership = C) } }
        assertEquals(contacts, f.contacts)
        assertArrayEquals(byteArrayOf(7), f.store.record(p, A)!!.envelope)
        f.confirm(p, membership = C, acknowledgeErasedWork = true)
        assertNull(f.store.record(p, A))
        assertEquals(C, EnterpriseVaultGenerations(f.files, f.keys).descriptor(EnterpriseVaultCodec.token(p))!!.lease.membershipId)
    }
    @Test fun clockAcrossContactCannotCommitARebootRollbackOrAlreadyExpiredAnchor(): Unit = runBlocking {
        for (change in listOf<(EnterpriseDeviceClock) -> EnterpriseDeviceClock>(
            { it.copy(boot = "boot-new", elapsedMillis = 0) },
            { it.copy(elapsedMillis = it.elapsedMillis - 1) },
            { it.copy(wallMillis = it.wallMillis - 1) },
            { it.copy(elapsedMillis = it.elapsedMillis + EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS) },
        )) {
            val f = VaultFixture(root.resolve(java.util.UUID.randomUUID().toString())); val p = partition()
            val store = ProtectedEnterprisePersistence(f.files, f.keys, f.fence, f.erasure, { f.identity }, { f.now }, EnterpriseContactAuthority { request ->
                f.now = change(f.now); testGrant(request.target.partition, A, 10000)
            })
            assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.confirm(p, store = store) } }
            assertTrue(f.contentKeys().isEmpty(), "An unusable request anchor must never commit")
        }
    }
    @Test fun longContactErasesExpiredPriorWorkAndCannotRestartItsClock(): Unit = runBlocking {
        val f = VaultFixture(root); val p = partition()
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        val store = ProtectedEnterprisePersistence(f.files, f.keys, f.fence, f.erasure, { f.identity }, { f.now }, EnterpriseContactAuthority { request ->
            f.now = f.now.copy(elapsedMillis = f.now.elapsedMillis + EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS)
            testGrant(request.target.partition, A, 10001)
        })
        assertThrows(EnterpriseVaultUnrecoverable::class.java) { runBlocking { f.confirm(p, store = store) } }
        assertTrue(f.contentKeys().isEmpty())
    }
    @Test fun sourceAccessEndedLocksAndPreservesUntilSameEnrollmentActiveProof(): Unit = runBlocking {
        val f = VaultFixture(root); val p = partition()
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        val store = ProtectedEnterprisePersistence(f.files, f.keys, f.fence, f.erasure, { f.identity }, { f.now }, EnterpriseContactAuthority { request ->
            EnterpriseContactReply.Lock(request.existingEnrollment!!, A, 10001)
        })
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.confirm(p, store = store) } }
        val recreated = VaultFixture(root, f.keys)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { recreated.store.record(p, A) } }
        recreated.server++
        recreated.confirm(p)
        assertArrayEquals(byteArrayOf(7), recreated.store.record(p, A)!!.envelope)
    }
    @Test fun observedRollbackRemainsLockedAfterNormalClockAndRestartUntilFreshContact(): Unit = runBlocking {
        val f = VaultFixture(root); val p = partition()
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        f.now = f.now.copy(wallMillis = f.now.wallMillis - 1)
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { f.store.record(p, A) } }
        f.now = f.now.copy(wallMillis = f.now.wallMillis + 1)
        val recreated = VaultFixture(root, f.keys); recreated.now = f.now
        assertThrows(EnterpriseLocalDenied::class.java) { runBlocking { recreated.store.record(p, A) } }
        recreated.server++
        recreated.confirm(p)
        assertArrayEquals(byteArrayOf(7), recreated.store.record(p, A)!!.envelope)
    }

    @Test fun legacyUnboundWorkCannotAcquireNewMembershipByContact(): Unit = runBlocking {
        val f = VaultFixture(root); val p = partition(); val token = EnterpriseVaultCodec.token(p)
        val key = f.keys.create(EnterpriseVaultCodec.alias(token), null)
        val legacy = VaultSnapshot(p, EnterpriseOfflineLease(A, 10000, f.now))
        legacy.entries["RECORD:$A"] = VaultEntry(VaultKind.RECORD, A, "", byteArrayOf(7))
        f.files.replace(token, EnterpriseVaultCodec.seal(legacy, key.secret))
        assertThrows(EnterpriseVaultUnrecoverable::class.java) { runBlocking { f.store.record(p, A) } }
        assertThrows(EnterpriseVaultUnrecoverable::class.java) { runBlocking { f.confirm(p) } }
        assertEquals(0, f.contacts)
        f.confirm(p, acknowledgeErasedWork = true)
        assertNull(f.store.record(p, A))
        assertNull(f.keys.existing(EnterpriseVaultCodec.alias(token)))
        assertNull(f.files.read(token))
        val bound = EnterpriseVaultGenerations(f.files, f.keys).descriptor(token)!!.lease
        assertEquals(B, bound.membershipId); assertEquals(C, bound.enrollmentId)
    }

}
