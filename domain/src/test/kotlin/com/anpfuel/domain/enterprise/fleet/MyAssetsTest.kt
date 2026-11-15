package com.anpfuel.domain.enterprise.fleet

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.sync.SyncProtocolException
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class MyAssetsTest {
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val member = id(8611)
    private val enrollment = id(8613)
    private val org = id(8600)
    private val received = EnterpriseDeviceClock("boot-1", 10_000, 1_763_208_000_000)

    private fun request(cursor: String? = null) = ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(
        id(8620), id(8621), "synthetic-reference-access", member, enrollment, cursor))

    private fun asset(n: Int, name: String, state: String = "ACTIVE", validTo: String? = null, fleet: String = "F-$n"): String =
        """{"id":"${id(n)}","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"$state","name":"$name","fleet_number":"$fleet","plate":"","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[{"id":"${id(n + 1)}","kind":"ODOMETER","modulus":null,"segment_id":"${id(n + 2)}","installed_at":"2026-11-01T08:00:00Z"}],"assignment":{"kind":"PRIMARY","valid_to":${validTo?.let { "\"$it\"" } ?: "null"}}}"""

    private fun reset(assets: List<String>, reason: String = "NEW_CURSOR", revision: Int = assets.size): ByteArray {
        val changes = assets.mapIndexed { index, value ->
            val entity = Regex("\"id\":\"([^\"]+)\"").find(value)!!.groupValues[1]
            """{"revision":${index + 1},"entity_type":"asset","entity_id":"$entity","op":"UPSERT","asset":$value}"""
        }
        return """{"schema_version":1,"request_id":"${id(8620)}","membership_id":"$member","enrollment_id":"$enrollment","mode":"RESET","reason":"$reason","from_revision":null,"revision":$revision,"confirmed_at":"2026-11-15T12:00:00Z","cursor":"cursor-1","entity_types":["asset"],"changes":[${changes.joinToString(",")}]}""".toByteArray()
    }

    private fun cache(vararg assets: String, reason: String = "NEW_CURSOR"): ReferenceCache {
        val request = request()
        val response = ReferenceFeedResponse.parse(reset(assets.toList(), reason, if (reason == "FORBIDDEN") 3 else assets.size), request, null)
        return ReferenceCache.apply(null, request, response, received)
    }

    @Test fun `confirmation uses elapsed time within one boot and wall time after a reboot`() {
        val day = MyAssets.CONFIRMATION_MILLIS
        assertTrue(MyAssets.confirmed(received, received))
        assertTrue(MyAssets.confirmed(received, received.copy(elapsedMillis = received.elapsedMillis + day - 1, wallMillis = 0)))
        assertFalse(MyAssets.confirmed(received, received.copy(elapsedMillis = received.elapsedMillis + day)))
        // Within one boot a moved wall clock neither confirms nor expires the list.
        assertTrue(MyAssets.confirmed(received, received.copy(elapsedMillis = received.elapsedMillis + 5, wallMillis = received.wallMillis + 10 * day)))
        assertFalse(MyAssets.confirmed(received, received.copy(elapsedMillis = received.elapsedMillis - 1)))
        val rebooted = EnterpriseDeviceClock("boot-2", 1, received.wallMillis + day - 1)
        assertTrue(MyAssets.confirmed(received, rebooted))
        assertFalse(MyAssets.confirmed(received, rebooted.copy(wallMillis = received.wallMillis + day)))
        assertFalse(MyAssets.confirmed(received, rebooted.copy(wallMillis = received.wallMillis - 1)))
    }

    @Test fun `presentation lists exactly the cached set with availability and assignment end facts`() {
        val ended = Instant.ofEpochMilli(received.wallMillis + 60_000)
        val snapshot = MyAssets.present(cache(asset(30, "zeta"), asset(40, "Alfa", state = "INACTIVE"),
            asset(50, "beta", validTo = ended.toString()), asset(60, "alfa", fleet = "A-1")), received)
        assertTrue(snapshot.listConfirmed); assertFalse(snapshot.forbidden)
        assertEquals(Instant.parse("2026-11-15T12:00:00Z"), snapshot.confirmedAt)
        assertEquals(listOf(id(60), id(40), id(50), id(30)), snapshot.assets.map { it.asset.id })
        val byId = snapshot.assets.associateBy { it.asset.id }
        assertFalse(byId.getValue(id(40)).available); assertTrue(byId.getValue(id(40)).reviewWarning)
        assertTrue(byId.getValue(id(40)).confirmed)
        assertFalse(byId.getValue(id(30)).reviewWarning)
        assertFalse(byId.getValue(id(50)).assignmentEnded); assertTrue(byId.getValue(id(50)).confirmed)

        val later = received.copy(elapsedMillis = received.elapsedMillis + 61_000, wallMillis = ended.toEpochMilli())
        val afterEnd = MyAssets.present(cache(asset(50, "beta", validTo = ended.toString())), later).assets.single()
        assertTrue(afterEnd.assignmentEnded); assertFalse(afterEnd.confirmed); assertTrue(afterEnd.reviewWarning)

        val stale = MyAssets.present(cache(asset(30, "zeta")), received.copy(elapsedMillis = received.elapsedMillis + MyAssets.CONFIRMATION_MILLIS))
        assertFalse(stale.listConfirmed)
        assertEquals(1, stale.assets.size, "The device clock never removes an asset")
        assertFalse(stale.assets.single().confirmed); assertTrue(stale.assets.single().reviewWarning)
    }

    @Test fun `forbidden caches hold nothing and the empty feed is a confirmed empty list`() {
        val forbidden = MyAssets.present(cache(reason = "FORBIDDEN"), received)
        assertTrue(forbidden.forbidden); assertTrue(forbidden.assets.isEmpty())
        val empty = MyAssets.present(cache(), received)
        assertFalse(empty.forbidden); assertTrue(empty.assets.isEmpty()); assertTrue(empty.listConfirmed)
    }

    @Test fun `the picker returns a hint and requires the review warning when the asset needs it`() {
        val snapshot = MyAssets.present(cache(asset(30, "Caminhão"), asset(40, "Trator", state = "DISPOSED")), received)
        val choice = requireNotNull(MyAssets.choose(org, snapshot, id(30), warningAccepted = false))
        assertEquals(org, choice.organizationId); assertEquals(id(30), choice.assetId); assertEquals(1, choice.assetRevision)
        assertEquals(AssetKind.VEHICLE, choice.kind); assertEquals(UsageBasis.DISTANCE, choice.usageBasis)
        assertEquals(listOf(id(31)), choice.meters.map { it.id }); assertEquals(snapshot.confirmedAt, choice.confirmedAt)
        assertFalse(choice.reviewWarningAccepted)
        assertNull(MyAssets.choose(org, snapshot, id(40), warningAccepted = false))
        assertTrue(requireNotNull(MyAssets.choose(org, snapshot, id(40), warningAccepted = true)).reviewWarningAccepted)
        assertNull(MyAssets.choose(org, snapshot, id(99), warningAccepted = true))
        assertThrows(IllegalArgumentException::class.java) { MyAssets.choose("not-an-org", snapshot, id(30), true) }
    }

    @Test fun `oversized responses and caches are refused as limits`() {
        val padded = reset(listOf(asset(30, "A"))).let { raw ->
            raw.copyOf(ReferenceFeedResponse.MAX_BYTES + 1).also { it.fill(' '.code.toByte(), raw.size, it.size) }
        }
        assertEquals("LIMIT", assertThrows(SyncProtocolException::class.java) { ReferenceFeedResponse.parse(padded, request(), null) }.code)
        val full = cache(*(0 until 200).map { asset(10_000 + 3 * it, "A") }.toTypedArray())
        assertEquals(200, full.assets.size)
        val request = request("cursor-1")
        val delta = """{"schema_version":1,"request_id":"${id(8620)}","membership_id":"$member","enrollment_id":"$enrollment","mode":"DELTA","reason":null,"from_revision":200,"revision":201,"confirmed_at":"2026-11-15T12:00:00Z","cursor":"cursor-2","entity_types":["asset"],"changes":[{"revision":201,"entity_type":"asset","entity_id":"${id(30)}","op":"UPSERT","asset":${asset(30, "B")}}]}""".toByteArray()
        val response = ReferenceFeedResponse.parse(delta, request, ReferenceCache.heldRevision(full, request))
        assertEquals("LIMIT", assertThrows(SyncProtocolException::class.java) { ReferenceCache.apply(full, request, response, received) }.code)
        assertEquals("LIMIT", assertThrows(SyncProtocolException::class.java) { ReferenceCache.decode(ByteArray(ReferenceCache.MAX_BYTES + 1)) }.code)
    }

    @Test fun `requests are canonical and refuse what the server would refuse`() {
        val raw = String(request("cursor-1").canonicalBytes)
        assertEquals("""{"access_token":"synthetic-reference-access","cursor":"cursor-1","enrollment_id":"$enrollment","family_id":"${id(8621)}","membership_id":"$member","request_id":"${id(8620)}","schema_version":1}""", raw)
        for (bad in listOf<() -> Unit>(
            { request("") }, { request("é") }, { request("x".repeat(1025)) },
            { ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot(id(1), id(2), "a b", member, enrollment, null)) },
            { ReferenceFeedRequest.create(ReferenceFeedRequestSnapshot("x", id(2), "token", member, enrollment, null)) },
        )) assertThrows(SyncProtocolException::class.java) { bad() }
    }

    @Test fun `cache state comparison and redaction`() {
        val first = cache(asset(30, "Caminhão secreto"))
        assertTrue(first.sameState(ReferenceCache.decode(first.canonicalBytes)))
        assertFalse(first.sameState(cache(asset(31, "Outro"))))
        assertFalse(first.sameState(null))
        for (value in listOf(first, first.assets.single(), request("cursor-1"), MyAssets.present(first, received),
            MyAssets.present(first, received).assets.single())) {
            val text = value.toString()
            assertFalse(text.contains("secreto") || text.contains(id(30)) || text.contains("cursor-1") || text.contains("synthetic"), text)
        }
    }
}
