package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.fleet.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.fleet.*
import com.anpfuel.domain.enterprise.sync.SyncProtocolException
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

private fun assetId(n: Int) = "7a000000-0000-4000-8000-" + n.toString().padStart(12, '0')
private const val ASSET_ORIGIN = "https://assets.example.invalid"

/** Server semantics of EMP-OFFL-006 (B-BR-EMP-339/340) over synthetic assets, emitting real wire bytes. */
internal class ReferenceServerFake {
    class Feed(var epoch: Int = 1, var revision: Long = 0, val log: MutableMap<String, Pair<Long, String?>> = linkedMapOf(),
        val current: MutableMap<String, String> = linkedMapOf(), var forbidden: Boolean = false)
    val feeds = mutableMapOf<String, Feed>()
    var pulls = 0
    var failNext: Exception? = null
    var notFound = 0
    var mutate: ((ByteArray) -> ByteArray)? = null

    fun asset(n: Int, name: String, state: String = "ACTIVE", validTo: String? = null): String =
        """{"id":"${assetId(n)}","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"$state","name":"$name","fleet_number":"F-$n","plate":"","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[],"assignment":{"kind":"PRIMARY","valid_to":${validTo?.let { "\"$it\"" } ?: "null"}}}"""
    fun feed(org: String) = feeds.getOrPut(org) { Feed() }
    fun assign(org: String, n: Int, name: String, state: String = "ACTIVE", validTo: String? = null) {
        feed(org).current[assetId(n)] = asset(n, name, state, validTo)
    }
    fun unassign(org: String, n: Int) { feed(org).current.remove(assetId(n)) }

    fun pull(attempt: EnterpriseQueueAttempt, request: ReferenceFeedRequest, held: Long?): EnterpriseReferencePull {
        pulls++
        failNext?.let { failNext = null; throw it }
        val sent = request.snapshot()
        if (notFound > 0) { notFound--; throw EnterpriseReferenceNotFound() }
        val feed = feed(attempt.partition.organizationId)
        val visible = if (feed.forbidden) emptyMap() else feed.current
        for (id in (visible.keys + feed.log.keys).sorted()) {
            val last = feed.log[id]; val now = visible[id]
            if (now != null && (last == null || last.second != now)) { feed.revision++; feed.log[id] = feed.revision to now }
            else if (now == null && last?.second != null) { feed.revision++; feed.log[id] = feed.revision to null }
        }
        val cursor = sent.cursor?.let { Regex("c-(\\d+)-(\\d+)").matchEntire(it)!!.groupValues }
        val (mode, reason, from) = when {
            feed.forbidden -> Triple("RESET", "FORBIDDEN", null)
            cursor == null -> Triple("RESET", "NEW_CURSOR", null)
            cursor[1].toInt() != feed.epoch -> Triple("RESET", "CURSOR_EXPIRED", null)
            cursor[2].toLong() > feed.revision -> Triple("RESET", "CURSOR_AHEAD", null)
            else -> Triple("DELTA", null, cursor[2].toLong())
        }
        val changes = if (feed.forbidden) emptyList() else feed.log.entries.sortedBy { it.value.first }
            .filter { (_, value) -> if (mode == "RESET") value.second != null else value.first > from!! }
            .map { (id, value) -> """{"revision":${value.first},"entity_type":"asset","entity_id":"$id","op":"${if (value.second == null) "DELETE" else "UPSERT"}","asset":${value.second ?: "null"}}""" }
        var raw = """{"schema_version":1,"request_id":"${sent.requestId}","membership_id":"${sent.membershipId}","enrollment_id":"${sent.enrollmentId}","mode":"$mode","reason":${reason?.let { "\"$it\"" } ?: "null"},"from_revision":${from ?: "null"},"revision":${feed.revision},"confirmed_at":"2026-11-15T12:00:${(pulls % 60).toString().padStart(2, '0')}Z","cursor":"c-${feed.epoch}-${feed.revision}","entity_types":["asset"],"changes":[${changes.joinToString(",")}]}""".toByteArray()
        mutate?.let { raw = it(raw) }
        return EnterpriseReferencePull(request, ReferenceFeedResponse.parse(raw, request, held))
    }
}

internal class MyAssetsFixture {
    val fence = EnterpriseSessionFence()
    val first = EnterpriseContextSummary(EnterprisePartition.of(assetId(1), ASSET_ORIGIN, assetId(2)), assetId(3), "Synthetic A")
    val second = EnterpriseContextSummary(EnterprisePartition.of(assetId(1), ASSET_ORIGIN, assetId(20)), assetId(30), "Synthetic B")
    var identity = EnterpriseLocalIdentity(assetId(1), ASSET_ORIGIN, assetId(9))
    var now = EnterpriseDeviceClock("boot-1", 1_000, 1_763_208_000_000)
    var deliveryCaptures = 0
    var failReplace: Exception? = null
    var replaceCalls = 0
    var raceOnReplace: (() -> Unit)? = null
    val server = ReferenceServerFake()
    /** Durable bytes per account/origin/organization partition, like the protected generation. */
    val disk = mutableMapOf<EnterprisePartition, ByteArray>()
    private var nonce = 100
    private fun membership(p: EnterprisePartition) = if (p.organizationId == first.partition.organizationId) first.membershipId else second.membershipId
    val source = object : EnterpriseQueueSource {
        override suspend fun account(): EnterpriseQueueAccountLease {
            val captured = identity; val epoch = fence.epoch; val background = fence.backgroundEpoch
            return EnterpriseQueueAccountLease.capture(captured, epoch, background, assetId(nonce++)) {
                identity == captured && fence.epoch == epoch && fence.backgroundEpoch == background
            }
        }
        override suspend fun deliveryAccount(): EnterpriseQueueAccountLease { deliveryCaptures++; return account() }
        override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
            account.assertCurrent()
            return account.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, membership(partition)), assetId(4), 1))
        }
    }
    val store = object : EnterpriseReferenceCacheStore {
        override suspend fun read(attempt: EnterpriseQueueAttempt): ReferenceCache? {
            attempt.assertCurrent()
            val cache = disk[attempt.partition]?.let { runCatching { ReferenceCache.decode(it) }.getOrNull() } ?: return null
            return cache.takeIf { it.membershipId == attempt.enrollment.target.membershipId && it.enrollmentId == attempt.enrollment.enrollmentId }
        }
        override suspend fun replace(attempt: EnterpriseQueueAttempt, expected: ReferenceCache?, next: ReferenceCache): Boolean {
            attempt.assertCurrent(); replaceCalls++
            raceOnReplace?.let { raceOnReplace = null; it() }
            failReplace?.let { failReplace = null; throw it }
            val stored = read(attempt)
            if (if (expected == null) stored != null else !expected.sameState(stored)) return false
            disk[attempt.partition] = next.canonicalBytes
            return true
        }
    }
    val transport = EnterpriseReferenceTransport { attempt, cursor, held ->
        attempt.assertCurrent()
        val session = identity
        server.pull(attempt, ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(assetId(nonce++), session.family,
            "synthetic-reference-access", attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId, cursor)), held)
    }
    val service = EnterpriseReferenceService(source, store, transport) { now }
    fun flow() = MyAssetsFlow(source, service, fence) { now }
    fun names(flow: MyAssetsFlow) = flow.state.value.snapshot?.assets?.map { it.asset.name }
}

class EnterpriseMyAssetsFlowTest {
    @Test fun `AC1 a reference pull shows exactly the assigned assets and a tombstone removes one`() = runTest {
        val f = MyAssetsFixture(); val org = f.first.partition.organizationId
        f.server.assign(org, 10, "Caminhão"); f.server.assign(org, 11, "Van")
        val flow = f.flow()
        flow.open(f.first, f.fence.epoch, online = true)
        val state = flow.state.value
        assertTrue(state.loaded); assertFalse(state.busy); assertFalse(state.refreshFailed); assertNull(state.error)
        assertEquals(listOf("Caminhão", "Van"), f.names(flow))
        assertTrue(state.snapshot!!.listConfirmed)
        f.server.unassign(org, 10); f.server.assign(org, 12, "Trator")
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertEquals(listOf("Trator", "Van"), f.names(flow))
        assertEquals(2, f.server.pulls)
        assertEquals(1, f.server.feed(org).epoch)
    }

    @Test fun `AC2 offline the protected list opens without network and keeps its confirmation time`() = runTest {
        val f = MyAssetsFixture(); val org = f.first.partition.organizationId
        f.server.assign(org, 10, "Caminhão")
        f.flow().open(f.first, f.fence.epoch, online = true)
        val pulls = f.server.pulls; val captures = f.deliveryCaptures
        // Another process: the server ends the assignment while this device has no network.
        f.server.unassign(org, 10)
        val offline = f.flow()
        offline.open(f.first, f.fence.epoch, online = false)
        assertEquals(pulls, f.server.pulls, "An offline open must not contact the feed")
        assertEquals(captures, f.deliveryCaptures)
        assertEquals(listOf("Caminhão"), f.names(offline), "Still listed: the server decides later")
        assertTrue(offline.state.value.snapshot!!.listConfirmed)
        f.now = f.now.copy(elapsedMillis = f.now.elapsedMillis + MyAssets.CONFIRMATION_MILLIS)
        offline.represent(f.fence.epoch, offline.state.value.generation)
        assertFalse(offline.state.value.snapshot!!.listConfirmed, "24 hours later the list is not confirmed")
        assertEquals(listOf("Caminhão"), f.names(offline))
        val choice = offline.choose(assetId(10), warningAccepted = false, f.fence.epoch, offline.state.value.generation)
        assertNull(choice, "A not confirmed asset needs the review warning first")
        val warned = requireNotNull(offline.choose(assetId(10), warningAccepted = true, f.fence.epoch, offline.state.value.generation))
        assertTrue(warned.reviewWarningAccepted); assertEquals(org, warned.organizationId)
        assertEquals(pulls, f.server.pulls, "Choosing never needs the network")
        // Reconnection: the tombstone removes it.
        offline.refresh(f.fence.epoch, offline.state.value.generation)
        assertEquals(emptyList<String>(), f.names(offline))
    }

    @Test fun `no cache offline is a loaded state that needs one connection and the empty feed is empty`() = runTest {
        val f = MyAssetsFixture()
        val flow = f.flow()
        flow.open(f.first, f.fence.epoch, online = false)
        assertTrue(flow.state.value.loaded); assertNull(flow.state.value.snapshot); assertEquals(0, f.server.pulls)
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertEquals(emptyList<String>(), f.names(flow)); assertFalse(flow.state.value.snapshot!!.forbidden)
    }

    @Test fun `AC3 an asset that is not available is chosen only after the warning`() = runTest {
        val f = MyAssetsFixture(); val org = f.first.partition.organizationId
        f.server.assign(org, 10, "Caminhão", state = "INACTIVE"); f.server.assign(org, 11, "Van")
        val flow = f.flow(); flow.open(f.first, f.fence.epoch, online = true)
        val generation = flow.state.value.generation
        assertNull(flow.choose(assetId(10), false, f.fence.epoch, generation))
        assertTrue(flow.choose(assetId(10), true, f.fence.epoch, generation)!!.reviewWarningAccepted)
        assertFalse(flow.choose(assetId(11), false, f.fence.epoch, generation)!!.reviewWarningAccepted)
        assertNull(flow.choose(assetId(11), false, f.fence.epoch + 1, generation), "An old gesture cannot choose")
        assertNull(flow.choose(assetId(99), true, f.fence.epoch, generation))
    }

    @Test fun `AC4 switching organization shows only that organization's cache`() = runTest {
        val f = MyAssetsFixture()
        f.server.assign(f.first.partition.organizationId, 10, "Caminhão A")
        f.server.assign(f.second.partition.organizationId, 20, "Escavadeira B")
        val flow = f.flow()
        flow.open(f.first, f.fence.epoch, online = true)
        assertEquals(listOf("Caminhão A"), f.names(flow))
        flow.open(f.second, f.fence.epoch, online = false)
        assertEquals(f.second, flow.state.value.context)
        assertNull(flow.state.value.snapshot, "Organization B has no cache yet; A's never shows under B")
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertEquals(listOf("Escavadeira B"), f.names(flow))
        flow.open(f.first, f.fence.epoch, online = false)
        assertEquals(listOf("Caminhão A"), f.names(flow))
        assertNull(flow.choose(assetId(20), true, f.fence.epoch, flow.state.value.generation), "B's asset cannot be chosen in A")
    }

    @Test fun `another account or an epoch change withdraws the list and the other account sees nothing`() = runTest {
        val f = MyAssetsFixture()
        f.server.assign(f.first.partition.organizationId, 10, "Caminhão")
        val flow = f.flow(); flow.open(f.first, f.fence.epoch, online = true)
        assertEquals(listOf("Caminhão"), f.names(flow))
        f.identity = EnterpriseLocalIdentity(assetId(77), ASSET_ORIGIN, assetId(78))
        f.fence.lock()
        flow.invalidateIfStale()
        assertNull(flow.state.value.snapshot); assertEquals(EnterpriseAccessError.STALE, flow.state.value.error)
        val other = EnterpriseContextSummary(EnterprisePartition.of(assetId(77), ASSET_ORIGIN, f.first.partition.organizationId),
            f.first.membershipId, "Synthetic A")
        val second = f.flow(); second.open(other, f.fence.epoch, online = false)
        assertTrue(second.state.value.loaded); assertNull(second.state.value.snapshot, "Another account on this device sees nothing")
        // The old owner cannot read its rendered row any more.
        assertNull(flow.choose(assetId(10), true, f.fence.epoch, flow.state.value.generation))
    }

    @Test fun `a failed refresh keeps the cached list and says it could not be confirmed now`() = runTest {
        val f = MyAssetsFixture(); val org = f.first.partition.organizationId
        f.server.assign(org, 10, "Caminhão")
        val flow = f.flow(); flow.open(f.first, f.fence.epoch, online = true)
        val before = f.disk[f.first.partition]!!.copyOf()
        f.server.assign(org, 11, "Van")
        f.server.failNext = com.anpfuel.application.enterprise.queue.EnterpriseQueueDeliveryFailure()
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.refreshFailed); assertEquals(listOf("Caminhão"), f.names(flow)); assertNull(flow.state.value.error)
        assertArrayEquals(before, f.disk[f.first.partition])
        // A malformed answer is refused whole: nothing applied.
        f.server.mutate = { String(it).replace("\"entity_types\":[\"asset\"]", "\"entity_types\":[\"asset\",\"trip\"]").toByteArray() }
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.refreshFailed); assertArrayEquals(before, f.disk[f.first.partition])
        f.server.mutate = null
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertFalse(flow.state.value.refreshFailed); assertEquals(listOf("Caminhão", "Van"), f.names(flow))
    }

    @Test fun `process death before the commit leaves the old cache and the next pull converges`() = runTest {
        val f = MyAssetsFixture(); val org = f.first.partition.organizationId
        f.server.assign(org, 10, "Caminhão")
        f.flow().open(f.first, f.fence.epoch, online = true)
        val before = f.disk[f.first.partition]!!.copyOf()
        f.server.unassign(org, 10); f.server.assign(org, 11, "Van")
        f.failReplace = EnterprisePersistenceUnavailable()
        val dying = f.flow(); dying.open(f.first, f.fence.epoch, online = true)
        assertTrue(dying.state.value.refreshFailed)
        assertArrayEquals(before, f.disk[f.first.partition], "Nothing of the interrupted refresh was committed")
        val restarted = f.flow(); restarted.open(f.first, f.fence.epoch, online = false)
        assertEquals(listOf("Caminhão"), f.names(restarted))
        restarted.refresh(f.fence.epoch, restarted.state.value.generation)
        assertEquals(listOf("Van"), f.names(restarted))
        assertEquals(org, f.server.feeds.keys.single())
    }

    @Test fun `a concurrent refresh that commits first wins and the loser shows the stored state`() = runTest {
        val f = MyAssetsFixture(); val org = f.first.partition.organizationId
        f.server.assign(org, 10, "Caminhão")
        val flow = f.flow(); flow.open(f.first, f.fence.epoch, online = true)
        f.server.assign(org, 11, "Van")
        f.raceOnReplace = {
            val stale = ReferenceCache.decode(f.disk[f.first.partition]!!)
            val request = ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(assetId(500), assetId(9), "synthetic-reference-access",
                f.first.membershipId, assetId(4), stale.cursor))
            val raw = """{"schema_version":1,"request_id":"${assetId(500)}","membership_id":"${f.first.membershipId}","enrollment_id":"${assetId(4)}","mode":"DELTA","reason":null,"from_revision":${stale.revision},"revision":${stale.revision},"confirmed_at":"2026-11-15T13:00:00Z","cursor":"c-1-${stale.revision}","entity_types":["asset"],"changes":[]}""".toByteArray()
            val other = ReferenceCache.apply(stale, request, ReferenceFeedResponse.parse(raw, request, stale.revision), f.now)
            f.disk[f.first.partition] = other.canonicalBytes
        }
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertEquals(listOf("Caminhão"), f.names(flow), "The winner's committed state is shown, not a mix")
        assertEquals(java.time.Instant.parse("2026-11-15T13:00:00Z"), flow.state.value.snapshot!!.confirmedAt)
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertEquals(listOf("Caminhão", "Van"), f.names(flow))
    }

    @Test fun `a cursor the server no longer verifies is retried once without a cursor`() = runTest {
        val f = MyAssetsFixture(); val org = f.first.partition.organizationId
        f.server.assign(org, 10, "Caminhão")
        val flow = f.flow(); flow.open(f.first, f.fence.epoch, online = true)
        f.server.assign(org, 11, "Van")
        f.server.notFound = 1
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertFalse(flow.state.value.refreshFailed); assertEquals(listOf("Caminhão", "Van"), f.names(flow))
        assertEquals(3, f.server.pulls)
        val before = f.disk[f.first.partition]!!.copyOf()
        f.server.notFound = 2
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.refreshFailed); assertEquals(5, f.server.pulls, "Exactly one retry")
        assertArrayEquals(before, f.disk[f.first.partition])
        // Without a held cursor there is nothing to retry.
        val fresh = f.flow(); f.server.notFound = 1
        fresh.open(f.second, f.fence.epoch, online = true)
        assertTrue(fresh.state.value.refreshFailed); assertEquals(6, f.server.pulls); assertNull(fresh.state.value.snapshot)
    }

    @Test fun `FORBIDDEN drops every cached asset and a reset replaces the set`() = runTest {
        val f = MyAssetsFixture(); val org = f.first.partition.organizationId
        f.server.assign(org, 10, "Caminhão")
        val flow = f.flow(); flow.open(f.first, f.fence.epoch, online = true)
        f.server.feed(org).forbidden = true
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.snapshot!!.forbidden); assertEquals(emptyList<String>(), f.names(flow))
        assertNull(flow.choose(assetId(10), true, f.fence.epoch, flow.state.value.generation))
        f.server.feed(org).forbidden = false; f.server.feed(org).epoch = 2
        flow.refresh(f.fence.epoch, flow.state.value.generation)
        assertFalse(flow.state.value.snapshot!!.forbidden); assertEquals(listOf("Caminhão"), f.names(flow))
    }

    @Test fun `detail is read only from the rendered list and protocol failures are not authority`() = runTest {
        val f = MyAssetsFixture(); val org = f.first.partition.organizationId
        f.server.assign(org, 10, "Caminhão")
        val flow = f.flow(); flow.open(f.first, f.fence.epoch, online = true)
        val generation = flow.state.value.generation
        flow.detail(assetId(10), f.fence.epoch, generation)
        assertEquals(assetId(10), flow.state.value.detail?.id)
        flow.backToList(f.fence.epoch, flow.state.value.generation)
        assertNull(flow.state.value.detail)
        flow.detail(assetId(99), f.fence.epoch, flow.state.value.generation)
        assertNull(flow.state.value.detail)
        assertThrows(SyncProtocolException::class.java) {
            ReferenceFeedResponse.parse("{}".toByteArray(), ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(assetId(1), assetId(2), "t",
                assetId(3), assetId(4), null)), null)
        }
    }
}
