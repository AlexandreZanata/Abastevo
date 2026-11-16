package com.anpfuel.app.ui.safety

import com.anpfuel.domain.safety.BlockReceipt
import com.anpfuel.domain.safety.BlockedUser
import com.anpfuel.domain.safety.CommunitySafetyException
import com.anpfuel.domain.safety.CommunitySafetyGateway
import com.anpfuel.domain.safety.SafetyContentRef
import com.anpfuel.domain.safety.UserReportReason
import com.anpfuel.domain.safety.UserReportReceipt
import java.util.UUID

/**
 * In-memory community safety server: content ids map to authors, blocks
 * are per author and the hidden question answers with the asked subset
 * written by blocked authors, like the backend (B-BR-SAFE-02..07).
 */
internal class FakeSafetyGateway : CommunitySafetyGateway {
    /** Content id (lowercase) to author key. */
    val authors = mutableMapOf<String, String>()
    val blockedAuthors = linkedSetOf<String>()
    val labels = mutableMapOf<String, String>()
    val reports = mutableListOf<Triple<SafetyContentRef, UserReportReason, String?>>()
    val hiddenQuestions = mutableListOf<List<SafetyContentRef>>()
    var listCalls = 0
    var reportReplayed = false
    var reportFailure: Exception? = null
    var blockFailure: Exception? = null
    var hiddenFailure: Exception? = null
    var listFailure: Exception? = null
    var removeFailure: Exception? = null

    fun blockIdOf(author: String): String = UUID.nameUUIDFromBytes(author.toByteArray()).toString()

    override suspend fun reportUser(ref: SafetyContentRef, reason: UserReportReason, detail: String?): UserReportReceipt {
        reportFailure?.let { throw it }
        if (authors[ref.id] == null) throw CommunitySafetyException.NotFound()
        reports += Triple(ref, reason, detail)
        return UserReportReceipt(UUID.randomUUID().toString(), reason, "2026-11-15T10:00:00Z", replayed = reportReplayed)
    }

    override suspend fun blockUser(ref: SafetyContentRef): BlockReceipt {
        blockFailure?.let { throw it }
        val author = authors[ref.id] ?: throw CommunitySafetyException.NotFound()
        val replayed = !blockedAuthors.add(author)
        return BlockReceipt(blockedUser(author, ref.kind), replayed)
    }

    override suspend fun listBlocks(): List<BlockedUser> {
        listCalls++
        listFailure?.let { throw it }
        return blockedAuthors.toList().asReversed().map { blockedUser(it, null) }
    }

    override suspend fun removeBlock(blockId: String) {
        removeFailure?.let { throw it }
        val author = blockedAuthors.firstOrNull { blockIdOf(it) == blockId } ?: throw CommunitySafetyException.NotFound()
        blockedAuthors.remove(author)
    }

    override suspend fun hiddenContent(refs: List<SafetyContentRef>): Set<SafetyContentRef> {
        hiddenQuestions += refs
        hiddenFailure?.let { throw it }
        return refs.filter { authors[it.id] in blockedAuthors }.toSet()
    }

    private fun blockedUser(author: String, kind: com.anpfuel.domain.safety.SafetyContentKind?) =
        BlockedUser(blockIdOf(author), labels[author].orEmpty(), kind, "2026-11-15T10:00:00Z")
}
