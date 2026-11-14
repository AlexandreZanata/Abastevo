package com.anpfuel.domain.enterprise.meter

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetId
import com.anpfuel.domain.enterprise.fleet.AssetRef
import com.anpfuel.domain.enterprise.fleet.FleetDomainException
import com.anpfuel.domain.enterprise.fleet.FleetFailure
import java.io.File
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

/** The independently written shared corpus supplies expectations; this adapter only names typed inputs/outputs. */
class EnterpriseMeterVectorsTest {
    private val aliases = mapOf("A" to 1, "B" to 2, "X" to 11, "Y" to 12, "M" to 21, "N" to 22, "S" to 31,
        "T" to 32, "E" to 41, "F" to 42, "D" to 51, "Q" to 52, "P" to 61)
    private fun id(index: Int) = "8c000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private fun alias(raw: String): String = when {
        raw == "?" -> ""
        raw in aliases -> id(aliases.getValue(raw))
        Regex("[ORF][0-9]+").matches(raw) -> id((if (raw[0] == 'O') 100 else if (raw[0] == 'R') 200 else 300) + raw.substring(1).toInt())
        else -> raw
    }
    private fun label(raw: String): String {
        aliases.entries.firstOrNull { id(it.value) == raw }?.let { return it.key }
        if (raw.startsWith("8c000000-0000-4000-8000-")) {
            val suffix = raw.substringAfterLast('-').toInt()
            if (suffix in 100..199) return "O" + (suffix - 100)
            if (suffix in 200..299) return "R" + (suffix - 200)
            if (suffix in 300..399) return "F" + (suffix - 300)
        }
        return raw
    }
    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map { val pair = it.split('=', limit = 2); require(pair.size == 2 && pair[0].isNotEmpty()); pair[0] to pair[1] }
        require(pairs.map { it.first }.toSet().size == pairs.size)
        return pairs.toMap()
    }
    private fun hex(raw: String) = raw.toByteArray(Charsets.UTF_8).joinToString("") { "%02x".format(it.toInt() and 255) }
    private fun decode(raw: String): String {
        require(raw.length % 2 == 0 && raw.all { it in '0'..'9' || it in 'a'..'f' })
        return Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT)
            .decode(ByteBuffer.wrap(raw.chunked(2).map { it.toInt(16).toByte() }.toByteArray())).toString()
    }
    private fun integer(raw: String): Long {
        if (raw.isEmpty() || raw.length > 19 || raw.length > 1 && raw[0] == '0' || raw.any { it !in '0'..'9' }) throw MeterDomainException()
        return raw.toLongOrNull() ?: throw MeterDomainException()
    }
    private fun modulus(args: Map<String, String>): Long? = args["modulus"]?.takeUnless { it == "?" }?.let(::integer)
    private fun segment(args: Map<String, String>): SegmentRef {
        val org = alias(args["org"] ?: "A")
        val asset = AssetRef(alias(args["asset_org"] ?: args["org"] ?: "A"), AssetId.parse(alias(args["asset"] ?: "X")))
        val meter = MeterRef(alias(args["meter_org"] ?: args["org"] ?: "A"), asset, alias(args["meter"] ?: "M"),
            MeterKind.parse(args["kind"] ?: "ODOMETER"), modulus(args))
        return SegmentRef(alias(args["segment_org"] ?: args["org"] ?: "A"), meter, alias(args["segment"] ?: "S"))
    }
    private fun reading(args: Map<String, String>): Reading {
        val org = alias(args["org"] ?: "A")
        val segment = segment(args)
        return Reading(ReadingRef(org, segment, alias(args["id"] ?: "R1")),
            MeterValues.parse(MeterKind.parse(args["value_kind"] ?: args["kind"] ?: "ODOMETER"), MeterEntryForm.BASE, args["value"] ?: "0"),
            TimeDeclaration(EnterpriseTime.parseInstant(args["occurred"] ?: "2026-11-02T00:00:00Z"), args["entry"] ?: "UTC",
                (args["offset"] ?: "0").toLongOrNull() ?: throw MeterDomainException()),
            EnterpriseTime.parseInstant(args["received"] ?: "2026-11-03T00:00:00Z"),
            MembershipRef(alias(args["member_org"] ?: args["org"] ?: "A"), alias(args["member"] ?: "E")),
            DeviceRef(alias(args["device_org"] ?: args["org"] ?: "A"), alias(args["device"] ?: "D")),
            ReadingOrigin.parse(args["origin"] ?: "STANDALONE"), ReadingMethod.parse(args["method"] ?: "TYPED"),
            OperationRef(alias(args["operation_org"] ?: args["org"] ?: "A"), alias(args["operation"] ?: "O1")),
            args["photo"]?.takeUnless { it == "?" }?.let { PhotoRef(alias(args["photo_org"] ?: args["org"] ?: "A"), alias(it)) },
            ReadingState.parse(args["state"] ?: "VALID"))
    }
    private val readingFields = setOf("org", "asset_org", "meter_org", "segment_org", "asset", "meter", "segment", "kind", "modulus",
        "id", "operation", "value", "value_kind", "occurred", "received", "state", "member", "member_org", "device", "device_org",
        "operation_org", "photo", "photo_org", "entry", "offset", "origin", "method")
    private fun opening(args: Map<String, String>): Reading {
        val input = args.filterKeys { it in readingFields }.toMutableMap()
        input += mapOf("id" to "R0", "operation" to "O0", "value" to "0", "state" to "VALID",
            "occurred" to "2026-11-01T00:00:00Z", "received" to "2026-11-01T00:01:00Z")
        for (field in readingFields) args["opening_$field"]?.let { input[field] = it }
        return reading(input)
    }
    private fun series(args: Map<String, String>): MeterSeries {
        val base = segment(args)
        var result = MeterSeries.open(base, opening(args))
        fun tuples(raw: String?): List<Reading> = if (raw.isNullOrEmpty()) emptyList() else raw.split('~').map {
            val parts = it.split(','); require(parts.size == 6)
            val input = args.filterKeys { key -> key in readingFields }.toMutableMap()
            input += mapOf("id" to parts[0], "operation" to parts[1], "value" to parts[2], "occurred" to parts[3], "received" to parts[4], "state" to parts[5])
            for (field in listOf("org", "asset", "meter", "segment", "kind")) args["reading_$field"]?.let { value -> input[field] = value }
            reading(input)
        }
        for (observation in tuples(args["readings"])) result = result.append(observation)
        val candidates = tuples(args["candidates"])
        require(candidates.map { it.reference.id }.distinct().size == candidates.size)
        for (raw in args["actions"]?.takeIf { it.isNotEmpty() }?.split('~').orEmpty()) {
            val parts = raw.split(','); require(parts.size in 5..6)
            val scoped = args.filterKeys { it in readingFields }.toMutableMap()
            for (field in listOf("org", "asset", "meter", "segment", "kind")) args["fact_$field"]?.let { scoped[field] = it }
            val factSegment = segment(scoped)
            val operation = OperationRef(alias(args["fact_operation_org"] ?: args["org"] ?: "A"), alias(parts[2]))
            val target = ReadingRef(base.organizationId, base, alias(parts[3]))
            val member = MembershipRef(alias(args["fact_member_org"] ?: args["org"] ?: "A"), alias(args["fact_member"] ?: "E"))
            val device = DeviceRef(alias(args["fact_device_org"] ?: args["org"] ?: "A"), alias(args["fact_device"] ?: "D"))
            val received = EnterpriseTime.parseInstant(args["fact_received"] ?: "2026-11-14T00:00:00Z")
            result = when (parts[0]) {
                "VOID" -> { require(parts.size == 5); result.void(VoidFact(factSegment, alias(parts[1]), operation, target, member, device, received, decode(parts[4]))) }
                "SUPERSEDE" -> {
                    require(parts.size == 6)
                    val replacement = candidates.firstOrNull { it.reference.id == alias(parts[4]) } ?: throw MeterDomainException()
                    result.supersede(SupersessionFact(factSegment, alias(parts[1]), operation, target, replacement.reference, member, device, received, decode(parts[5])), replacement)
                }
                else -> error("Unknown oracle correction action")
            }
        }
        return result
    }
    private fun execute(operation: String, args: Map<String, String>): Map<String, String> = when (operation) {
        "VALUE" -> MeterValues.parse(MeterKind.parse(args.getValue("kind")), MeterEntryForm.parse(args.getValue("form")),
            decode(args.getValue("text_hex")), args["minutes_hex"]?.let(::decode), modulus(args)).let {
            mapOf("kind" to it.kind.name, "unit" to it.kind.unit, "value" to it.units.toString())
        }
        "METER" -> {
            val meter = segment(args)
            val opening = opening(args)
            MeterSeries.open(meter, opening)
            mapOf("kind" to meter.meter.kind.name, "unit" to meter.meter.kind.unit, "modulus" to (meter.meter.modulus?.toString() ?: "?"),
                "opening_value" to opening.value.units.toString(), "opening_state" to opening.classification.name)
        }
        "OPEN_SET" -> {
            val org = alias(args["org"] ?: "A")
            val meters = args.getValue("meters").takeIf { it.isNotEmpty() }?.split('~')?.map {
                val p = it.split(','); require(p.size == 3)
                MeterRef(org, AssetRef(org, AssetId.parse(alias(p[1]))), alias(p[0]), MeterKind.parse(p[2]))
            }.orEmpty()
            val open = MeterOpenSet.validate(alias(args["request_org"] ?: args["org"] ?: "A"), meters)
            mapOf("count" to open.size.toString(), "kinds" to open.map { label(it.asset.assetId.value) + ":" + it.kind.name }.sorted().joinToString(","))
        }
        "READING" -> reading(args).let {
            mapOf("org" to label(it.reference.organizationId), "asset" to label(it.reference.segment.meter.asset.assetId.value),
                "meter" to label(it.reference.segment.meter.id), "segment" to label(it.reference.segment.id), "id" to label(it.reference.id),
                "member" to label(it.author.id), "device" to label(it.device.id), "operation" to label(it.operation.id), "photo" to (it.photo?.id?.let(::label) ?: "?"),
                "kind" to it.value.kind.name, "unit" to it.value.kind.unit, "value" to it.value.units.toString(), "state" to it.classification.name,
                "occurred" to EnterpriseTime.formatInstant(it.declared.occurredAt), "received" to EnterpriseTime.formatInstant(it.receivedAt),
                "entry" to it.declared.entryZone, "offset" to it.declared.deviceOffsetSeconds.toString(), "origin" to it.origin.name, "method" to it.method.name)
        }
        "PRESENCE" -> {
            val kind = MeterKind.parse(args.getValue("kind"))
            if (args.getValue("value") == "?") {
                val missing = MeterPresence.missing(decode(args.getValue("reason_hex")))
                mapOf("kind" to kind.name, "presence" to "MISSING", "value" to "?", "reason_hex" to hex(missing.reason))
            } else {
                val observed = MeterPresence.observed(MeterValues.parse(kind, MeterEntryForm.BASE, args.getValue("value")))
                mapOf("kind" to observed.value.kind.name, "presence" to "OBSERVED", "value" to observed.value.units.toString(), "reason_hex" to "")
            }
        }
        "HISTORY" -> {
            val series = series(args)
            val readings = series.ordered()
            val latest = series.latest(EnterpriseTime.parseInstant(args["at"] ?: "2026-11-30T00:00:00Z"))
            mapOf("order" to readings.joinToString(",") { label(it.reference.id) }, "states" to readings.joinToString(",") { label(it.reference.id) + ":" + series.state(it.reference).name },
                "values" to readings.joinToString(",") { label(it.reference.id) + ":" + it.value.units }, "latest" to (latest?.reference?.id?.let(::label) ?: "?"),
                "latest_value" to (latest?.value?.units?.toString() ?: "?"), "count" to readings.size.toString(), "facts" to series.facts.size.toString())
        }
        "USAGE" -> series(args).usage(EnterpriseTime.parseInstant(args.getValue("start")), EnterpriseTime.parseInstant(args.getValue("end"))).let {
            mapOf("kind" to it.kind.name, "unit" to it.kind.unit, "status" to it.status.name, "usage" to (it.value?.units?.toString() ?: "?"),
                "from" to (it.from?.reference?.id?.let(::label) ?: "?"), "to" to (it.to?.reference?.id?.let(::label) ?: "?"), "reason" to it.reason)
        }
        else -> error("Unknown shared meter operation")
    }

    @TestFactory fun allSharedMeterVectors(): List<DynamicTest> {
        val file = File("../contracts/testdata/enterprise/meter/meter-v1.psv")
        assertEquals("578b99a926e5591aea9bcf5ed98d6e7529301c672c51e6353d87c09f3f55147a",
            MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        assertEquals(261, rows.size)
        require(rows.all { it.size == 4 && it[0].isNotEmpty() })
        assertEquals(rows.size, rows.map { it[0] }.toSet().size)
        assertEquals(mapOf("VALUE" to 99, "METER" to 14, "OPEN_SET" to 8, "READING" to 54, "PRESENCE" to 11, "HISTORY" to 56, "USAGE" to 19), rows.groupingBy { it[1] }.eachCount())
        return rows.map { (id, operation, rawArgs, expected) -> DynamicTest.dynamicTest(id) {
            val result = try { execute(operation, fields(rawArgs)) }
            catch (error: MeterDomainException) { mapOf("error" to error.failure.name) }
            catch (_: EnterpriseTimeException) { mapOf("error" to "INVALID") }
            catch (error: FleetDomainException) { mapOf("error" to if (error.failure == FleetFailure.SCOPE) "SCOPE" else "INVALID") }
            assertEquals(fields(expected), result)
        } }
    }
}
