package com.anpfuel.application.usecase.safety

import com.anpfuel.domain.safety.BlockReceipt
import com.anpfuel.domain.safety.BlockedUser
import com.anpfuel.domain.safety.CommunitySafetyException
import com.anpfuel.domain.safety.CommunitySafetyGateway
import com.anpfuel.domain.safety.SafetyContentKind
import com.anpfuel.domain.safety.SafetyContentRef
import com.anpfuel.domain.safety.UserReportReason
import com.anpfuel.domain.safety.UserReportReceipt
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class CommunitySafetyUseCasesTest {

    private class FakeGateway : CommunitySafetyGateway {
        var blocks = mutableListOf<BlockedUser>()
        var hiddenIds = mutableSetOf<String>()
        var listCalls = 0
        val hiddenBatches = mutableListOf<List<SafetyContentRef>>()
        var reported: Triple<SafetyContentRef, UserReportReason, String?>? = null
        var hiddenFailure: Exception? = null
        var hiddenFailsAfterBatches = Int.MAX_VALUE
        var listFailure: Exception? = null
        var removeFailure: Exception? = null
        var blockReplayed = false
        var hang = false

        override suspend fun reportUser(ref: SafetyContentRef, reason: UserReportReason, detail: String?): UserReportReceipt {
            reported = Triple(ref, reason, detail)
            return UserReportReceipt("r-1", reason, "2026-11-15T10:00:00Z", replayed = false)
        }

        override suspend fun blockUser(ref: SafetyContentRef): BlockReceipt {
            val user = BlockedUser(newId(blocks.size), "Ana", ref.kind, "2026-11-15T10:00:00Z")
            if (!blockReplayed) blocks.add(0, user)
            return BlockReceipt(user, blockReplayed)
        }

        override suspend fun listBlocks(): List<BlockedUser> {
            listCalls++
            listFailure?.let { throw it }
            return blocks.toList()
        }

        override suspend fun removeBlock(blockId: String) {
            removeFailure?.let { throw it }
            blocks.removeAll { it.blockId == blockId }
        }

        override suspend fun hiddenContent(refs: List<SafetyContentRef>): Set<SafetyContentRef> {
            if (hang) awaitCancellation()
            if (hiddenBatches.size >= hiddenFailsAfterBatches) hiddenFailure?.let { throw it }
            hiddenBatches += refs
            if (hiddenFailsAfterBatches == Int.MAX_VALUE) hiddenFailure?.let { throw it }
            // A misbehaving server answer outside the question must be ignored.
            return refs.filter { it.id in hiddenIds }.toSet() + SafetyContentRef(SafetyContentKind.FORUM_POST, newId(999))
        }
    }

    private var account: String? = "account-1"
    private val gateway = FakeGateway()
    private val state = CommunityBlockState(gateway) { account }
    private val filter = FilterHiddenContentUseCase(gateway, state)

    private fun ref(n: Int, kind: SafetyContentKind = SafetyContentKind.FORUM_COMMENT) = SafetyContentRef(kind, newId(n))

    @Test
    fun `report user sends the normalized detail and the reason`() = runTest {
        val receipt = ReportCommunityUserUseCase(gateway)(ref(1), UserReportReason.HATE, "  detail\r\n ")
        assertEquals(UserReportReason.HATE, receipt.reason)
        assertEquals(Triple(ref(1), UserReportReason.HATE, "detail"), gateway.reported)
    }

    @Test
    fun `report user omits a blank detail`() = runTest {
        ReportCommunityUserUseCase(gateway)(ref(1), UserReportReason.SPAM, "   ")
        assertNull(gateway.reported!!.third)
    }

    @Test
    fun `report user refuses a long detail and a malformed reference locally`() {
        val useCase = ReportCommunityUserUseCase(gateway)
        val tooLong = assertThrows(CommunitySafetyException.BadRequest::class.java) {
            runBlocking { useCase(ref(1), UserReportReason.OTHER, "a".repeat(201)) }
        }
        assertEquals("safety.detail-too-long", tooLong.code)
        val badId = assertThrows(CommunitySafetyException.BadRequest::class.java) {
            runBlocking { useCase(SafetyContentRef(SafetyContentKind.FORUM_POST, "x"), UserReportReason.OTHER) }
        }
        assertEquals("safety.bad-id", badId.code)
        assertNull(gateway.reported)
    }

    @Test
    fun `no blocks means the hidden question is never asked`() = runTest {
        assertEquals(emptySet<SafetyContentRef>(), filter(listOf(ref(1), ref(2))))
        assertEquals(emptySet<SafetyContentRef>(), filter(listOf(ref(3))))
        assertEquals(1, gateway.listCalls, "the block list loads once per account")
        assertTrue(gateway.hiddenBatches.isEmpty())
    }

    @Test
    fun `visitors never ask anything`() = runTest {
        account = null
        assertEquals(emptySet<SafetyContentRef>(), filter(listOf(ref(1))))
        assertEquals(0, gateway.listCalls)
        assertTrue(gateway.hiddenBatches.isEmpty())
    }

    @Test
    fun `a block switches the question on and only asked references come back`() = runTest {
        filter(listOf(ref(1)))
        BlockCommunityUserUseCase(gateway, state)(ref(1))
        gateway.hiddenIds += ref(1).id
        val hidden = filter(listOf(ref(1), ref(2), ref(1)))
        assertEquals(setOf(ref(1)), hidden)
        assertEquals(listOf(listOf(ref(1), ref(2))), gateway.hiddenBatches, "duplicates are asked once")
        assertEquals(1, gateway.listCalls)
        assertEquals(1L, state.revisions.value)
    }

    @Test
    fun `references are asked in batches of at most one hundred and malformed ones are skipped`() = runTest {
        gateway.blocks += BlockedUser(newId(500), "", null, "2026-11-15T10:00:00Z")
        val refs = (1..250).map { ref(it) } + SafetyContentRef(SafetyContentKind.FORUM_COMMENT, "not-a-uuid")
        gateway.hiddenIds += ref(150).id
        val hidden = filter(refs)
        assertEquals(listOf(100, 100, 50), gateway.hiddenBatches.map { it.size })
        assertEquals(setOf(ref(150)), hidden)
    }

    @Test
    fun `refusals fail open and keep what earlier batches found`() = runTest {
        gateway.blocks += BlockedUser(newId(500), "", null, "2026-11-15T10:00:00Z")
        gateway.hiddenIds += ref(5).id
        gateway.hiddenFailure = CommunitySafetyException.Unavailable()
        gateway.hiddenFailsAfterBatches = 1
        val hidden = filter((1..150).map { ref(it) })
        assertEquals(setOf(ref(5)), hidden)

        gateway.hiddenBatches.clear()
        gateway.hiddenFailsAfterBatches = Int.MAX_VALUE
        gateway.hiddenFailure = CommunitySafetyException.Offline()
        assertEquals(emptySet<SafetyContentRef>(), filter(listOf(ref(5))))
    }

    @Test
    fun `a slow answer fails open within the budget`() = runTest {
        gateway.blocks += BlockedUser(newId(500), "", null, "2026-11-15T10:00:00Z")
        gateway.hang = true
        assertEquals(emptySet<SafetyContentRef>(), FilterHiddenContentUseCase(gateway, state, budgetMillis = 50)(listOf(ref(1))))
    }

    @Test
    fun `an unreadable block list still asks and fails open`() = runTest {
        gateway.listFailure = CommunitySafetyException.Offline()
        assertTrue(state.mayHaveBlocks())
        gateway.hiddenFailure = CommunitySafetyException.Offline()
        assertEquals(emptySet<SafetyContentRef>(), filter(listOf(ref(1))))
        assertEquals(2, gateway.listCalls, "an unknown count is retried later")
    }

    @Test
    fun `unblocking the last block stops the question without reloading`() = runTest {
        val receipt = BlockCommunityUserUseCase(gateway, state)(ref(1))
        assertTrue(state.mayHaveBlocks())
        assertEquals(1, gateway.listCalls)
        UnblockCommunityUserUseCase(gateway, state)(receipt.blockedUser.blockId.uppercase())
        assertFalse(state.mayHaveBlocks())
        assertEquals(1, gateway.listCalls)
        assertEquals(2L, state.revisions.value)
    }

    @Test
    fun `counts follow writes once known and replays do not count twice`() = runTest {
        assertFalse(state.mayHaveBlocks())
        gateway.blockReplayed = true
        BlockCommunityUserUseCase(gateway, state)(ref(1))
        assertFalse(state.mayHaveBlocks(), "a replayed block adds nothing")
        gateway.blockReplayed = false
        BlockCommunityUserUseCase(gateway, state)(ref(2))
        assertTrue(state.mayHaveBlocks())
        assertEquals(1, gateway.listCalls)
    }

    @Test
    fun `a listed page is the exact count`() = runTest {
        gateway.blocks += BlockedUser(newId(500), "Ana", SafetyContentKind.STATION_COMMENT, "2026-11-15T10:00:00Z")
        val listed = ListBlockedUsersUseCase(gateway, state)()
        assertEquals(1, listed.size)
        assertTrue(state.mayHaveBlocks())
        assertEquals(1, gateway.listCalls, "the list screen already loaded the count")
    }

    @Test
    fun `switching accounts never reuses the previous count`() = runTest {
        gateway.blocks += BlockedUser(newId(500), "", null, "2026-11-15T10:00:00Z")
        assertTrue(state.mayHaveBlocks())
        account = "account-2"
        gateway.blocks.clear()
        assertFalse(state.mayHaveBlocks())
        assertEquals(2, gateway.listCalls)
    }

    @Test
    fun `unblock validates the id and recounts after a missing block`() {
        val useCase = UnblockCommunityUserUseCase(gateway, state)
        assertThrows(CommunitySafetyException.BadRequest::class.java) { runBlocking { useCase("../x") } }
        gateway.removeFailure = CommunitySafetyException.NotFound()
        runBlocking { assertFalse(state.mayHaveBlocks()) }
        assertThrows(CommunitySafetyException.NotFound::class.java) { runBlocking { useCase(newId(7)) } }
        gateway.blocks += BlockedUser(newId(8), "", null, "2026-11-15T10:00:00Z")
        runBlocking { assertTrue(state.mayHaveBlocks()) }
        assertEquals(2, gateway.listCalls)
    }

    @Test
    fun `block refuses a malformed reference locally`() {
        assertThrows(CommunitySafetyException.BadRequest::class.java) {
            runBlocking { BlockCommunityUserUseCase(gateway, state)(SafetyContentRef(SafetyContentKind.STATION_COMMENT, "")) }
        }
        assertTrue(gateway.blocks.isEmpty())
    }

    @Test
    fun `an empty question asks nothing`() = runTest {
        assertEquals(emptySet<SafetyContentRef>(), filter(emptyList()))
        assertEquals(0, gateway.listCalls)
    }

    private companion object {
        fun newId(n: Int): String = "00000000-0000-4000-8000-%012d".format(n)
    }
}
