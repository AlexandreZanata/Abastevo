package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.ReportingZones
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetId
import com.anpfuel.domain.enterprise.fleet.AssetRef
import com.anpfuel.domain.enterprise.fleet.FleetDomainException
import com.anpfuel.domain.enterprise.meter.MeterDomainException
import com.anpfuel.domain.enterprise.meter.MeterEntryForm
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.meter.MeterPresence
import com.anpfuel.domain.enterprise.meter.MeterRef
import com.anpfuel.domain.enterprise.meter.MeterValues
import com.anpfuel.domain.enterprise.meter.ReadingRef
import com.anpfuel.domain.enterprise.meter.SegmentRef
import java.io.File
import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** Loads the same trip-v1 oracle as Go; expectations are never computed here. */
class EnterpriseTripVectorsTest {
    private val corpusHash = "068f9dbb8a7a994fdb1c424365d2c45b06655ec01a84b29b874253ea86c6d295"
    private val wantCounts = mapOf("ENUM" to 49, "TRANSITION" to 40, "STANDING" to 14, "START" to 57, "FINISH" to 47)
    private val errorCodes = TripFailure.entries.map { it.name }.toSet()
    private val aliases = mapOf("A" to 1, "B" to 2, "T" to 11, "T2" to 12, "V" to 21, "V2" to 22, "D" to 31, "D2" to 32,
        "M" to 41, "M2" to 42, "MT" to 51, "MT2" to 52, "SG" to 61, "SG2" to 62, "R1" to 71, "R2" to 72, "R3" to 73)
    private val startKeys = ("scope org id id_org author author_org asset asset_org driver driver_org dep dep_org dep_asset " +
        "dep_meter dep_segment dep_reading dep_kind dep_value_kind dep_value dep_modulus dep_reason_hex at entry offset zone unit_zone omit")
    private val finishKeys = "$startKeys fin_scope fin_author fin_author_org fin fin_org fin_asset fin_meter fin_segment fin_reading " +
        "fin_kind fin_value_kind fin_value fin_modulus fin_reason_hex fin_at fin_entry fin_offset fin_omit again"
    private val allowedArgs = mapOf("ENUM" to "kind value_hex", "TRANSITION" to "state command", "STANDING" to "operational review",
        "START" to startKeys, "FINISH" to finishKeys).mapValues { it.value.split(' ').toSet() }
    private val startOmittable = setOf("id", "author", "asset", "driver", "departure", "declaration", "zone")
    private val finishOmittable = setOf("author", "final", "declaration")
    private val enumSelectors = setOf("STATE", "COMMAND", "REVIEW")
    private val departureReasonHex = hex("Odometro ilegivel")
    private val finalReasonHex = hex("Sem leitura final")

    private fun hex(raw: String): String = raw.toByteArray(Charsets.UTF_8).joinToString("") { "%02x".format(it.toInt() and 255) }
    private fun id(index: Int) = "5e000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private fun alias(raw: String): String = aliases[raw]?.let(::id) ?: raw
    private fun label(raw: String): String = aliases.entries.firstOrNull { id(it.value) == raw }?.key ?: raw
    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotBlank()); p[0] to p[1] }
        require(pairs.map { it.first }.distinct().size == pairs.size)
        return pairs.toMap()
    }
    private fun decode(raw: String): String {
        require(raw.length % 2 == 0 && raw.all { it in '0'..'9' || it in 'a'..'f' })
        return Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT)
            .decode(ByteBuffer.wrap(raw.chunked(2).map { it.toInt(16).toByte() }.toByteArray())).toString()
    }
    private fun caseShape(operation: String, args: Map<String, String>) {
        val allowed = requireNotNull(allowedArgs[operation])
        require(args.keys.all { it in allowed })
        if (operation == "ENUM") require(args.getValue("kind") in enumSelectors)
    }
    private fun present(raw: String?): String? = raw?.takeUnless { it == "?" }
    private fun omitted(raw: String?, allowed: Set<String>): Set<String> {
        if (raw.isNullOrEmpty()) return emptySet()
        val keys = raw.split(',')
        require(keys.all { it in allowed } && keys.distinct().size == keys.size)
        return keys.toSet()
    }

    @Test fun `malformed fixture grammar is rejected before domain dispatch`() {
        assertThrows(IllegalArgumentException::class.java) { caseShape("ENUM", fields("kind=OTHER;value_hex=")) }
        assertThrows(IllegalArgumentException::class.java) { caseShape("OTHER", fields("state=PAUSED")) }
        assertThrows(IllegalArgumentException::class.java) { caseShape("STANDING", fields("operational=COMPLETED;extra=1")) }
        assertThrows(IllegalArgumentException::class.java) { fields("a=1;a=2") }
        assertThrows(IllegalArgumentException::class.java) { fields("novalue") }
        assertThrows(IllegalArgumentException::class.java) { fields("") }
        assertThrows(IllegalArgumentException::class.java) { omitted("asset,asset", startOmittable) }
        assertThrows(IllegalArgumentException::class.java) { omitted("final", startOmittable) }
        assertThrows(IllegalArgumentException::class.java) { decode("4F") }
        assertThrows(CharacterCodingException::class.java) { decode("ff") }
        assertThrows(CharacterCodingException::class.java) { decode("eda080") }
    }

    /** Builds departure ("dep") or final ("fin") evidence in the Go fixture order. */
    private fun evidence(f: Map<String, String>, prefix: String): TripMeterEvidence = when (f.getValue(prefix)) {
        "ABSENT" -> TripMeterEvidence.Absent(MeterPresence.missing(decode(f.getValue("${prefix}_reason_hex"))))
        "READING" -> {
            val org = alias(f.getValue("${prefix}_org"))
            val asset = AssetRef(org, AssetId.parse(alias(f.getValue("${prefix}_asset"))))
            val modulus = present(f["${prefix}_modulus"])?.let { requireNotNull(it.toLongOrNull()) }
            val meter = MeterRef(org, asset, alias(f.getValue("${prefix}_meter")), MeterKind.parse(f.getValue("${prefix}_kind")), modulus)
            val segment = SegmentRef(org, meter, alias(f.getValue("${prefix}_segment")))
            val reference = ReadingRef(org, segment, alias(f.getValue("${prefix}_reading")))
            val value = MeterValues.parse(MeterKind.parse(f.getValue("${prefix}_value_kind")), MeterEntryForm.BASE, f.getValue("${prefix}_value"))
            TripMeterEvidence.Observed(reference, value)
        }
        else -> error("Unknown evidence fixture mode")
    }

    private fun declaration(at: String, entry: String, offset: String): TimeDeclaration =
        TimeDeclaration(EnterpriseTime.parseInstant(at), entry, offset.toLongOrNull() ?: throw TripDomainException())

    private fun startFields(args: Map<String, String>): Map<String, String> {
        val f = mutableMapOf("org" to "A", "id" to "T", "author" to "M", "asset" to "V", "driver" to "D", "dep" to "READING",
            "dep_meter" to "MT", "dep_segment" to "SG", "dep_reading" to "R1", "dep_kind" to "ODOMETER", "dep_value" to "184320000",
            "dep_reason_hex" to departureReasonHex, "at" to "2026-11-14T12:00:00Z", "entry" to "America/Cuiaba", "offset" to "-14400",
            "zone" to "America/Cuiaba", "unit_zone" to "?")
        f.putAll(args)
        val org = f.getValue("org")
        for (key in listOf("scope", "id_org", "author_org", "asset_org", "driver_org", "dep_org")) f.putIfAbsent(key, org)
        f.putIfAbsent("dep_asset", f.getValue("asset"))
        f.putIfAbsent("dep_value_kind", f.getValue("dep_kind"))
        f.putIfAbsent("dep_modulus", "?")
        return f
    }

    private fun start(f: Map<String, String>): Trip {
        val omit = omitted(f["omit"], startOmittable)
        val reference = if ("id" in omit) null else TripRef(alias(f.getValue("id_org")), alias(f.getValue("id")))
        val author = if ("author" in omit) null else MembershipRef(alias(f.getValue("author_org")), alias(f.getValue("author")))
        val asset = if ("asset" in omit) null else AssetId.parse(alias(f.getValue("asset"))).let { AssetRef(alias(f.getValue("asset_org")), it) }
        val driver = if ("driver" in omit) null else EmployeeRef(alias(f.getValue("driver_org")), alias(f.getValue("driver")))
        val departure = if ("departure" in omit) null else evidence(f, "dep")
        val declared = if ("declaration" in omit) null else declaration(f.getValue("at"), f.getValue("entry"), f.getValue("offset"))
        val zones = if ("zone" in omit) null else ReportingZones(f.getValue("zone"), present(f["unit_zone"]))
        return Trip.start(alias(f.getValue("scope")), TripStartInput(reference, author, asset, driver, departure, declared, zones))
    }

    private fun finish(args: Map<String, String>): Trip {
        val f = startFields(args).toMutableMap()
        val defaults = mapOf("fin_scope" to f.getValue("scope"), "fin_author" to "M", "fin_author_org" to f.getValue("org"),
            "fin" to "READING", "fin_org" to f.getValue("org"), "fin_asset" to f.getValue("asset"), "fin_meter" to f.getValue("dep_meter"),
            "fin_segment" to f.getValue("dep_segment"), "fin_reading" to "R2", "fin_kind" to f.getValue("dep_kind"),
            "fin_value" to "184610000", "fin_modulus" to f.getValue("dep_modulus"), "fin_reason_hex" to finalReasonHex,
            "fin_at" to "2026-11-14T15:00:00Z", "fin_entry" to "America/Cuiaba", "fin_offset" to "-14400")
        defaults.forEach { (k, v) -> f.putIfAbsent(k, v) }
        f.putIfAbsent("fin_value_kind", f.getValue("fin_kind"))
        val started = start(f)
        val omit = omitted(f["fin_omit"], finishOmittable)
        val author = if ("author" in omit) null else MembershipRef(alias(f.getValue("fin_author_org")), alias(f.getValue("fin_author")))
        val final = if ("final" in omit) null else evidence(f, "fin")
        val declared = if ("declaration" in omit) null else declaration(f.getValue("fin_at"), f.getValue("fin_entry"), f.getValue("fin_offset"))
        val input = TripFinishInput(author, final, declared)
        val again = f["again"] ?: "false"
        require(again == "true" || again == "false")
        val before = tripFields(started)
        val scope = alias(f.getValue("fin_scope"))
        val next = try { started.finish(scope, input) } finally { assertEquals(before, tripFields(started), "finish changed its input trip") }
        return if (again == "true") next.finish(scope, input) else next
    }

    private fun evidenceField(evidence: TripMeterEvidence?): String = when (evidence) {
        is TripMeterEvidence.Observed -> "${label(evidence.reference.id)}:${evidence.value.kind.name}:${evidence.value.units}"
        is TripMeterEvidence.Absent -> "ABSENT:${hex(evidence.missing.reason)}"
        null -> "?"
    }

    private fun tripFields(trip: Trip): Map<String, String> {
        val start = trip.started
        val end = trip.finished
        val usage = trip.meterUsage
        return mapOf("ready" to "true", "org" to label(trip.organizationId), "id" to label(trip.reference.id),
            "asset" to label(trip.asset.assetId.value), "driver" to label(trip.driver.id), "state" to trip.state.name,
            "start_author" to label(start.author.id), "start_at" to EnterpriseTime.formatInstant(start.declared.occurredAt),
            "start_entry" to start.declared.entryZone, "start_offset" to start.declared.deviceOffsetSeconds.toString(),
            "departure" to evidenceField(start.evidence), "day" to trip.reporting.day, "zone" to trip.reporting.zone,
            "reporting_source" to trip.reporting.source, "finish_author" to (end?.author?.id?.let(::label) ?: "?"),
            "finish_at" to (end?.declared?.occurredAt?.let(EnterpriseTime::formatInstant) ?: "?"),
            "finish_entry" to (end?.declared?.entryZone ?: "?"), "finish_offset" to (end?.declared?.deviceOffsetSeconds?.toString() ?: "?"),
            "final" to evidenceField(end?.evidence), "usage" to usage.status.name, "usage_reason" to (usage.reason?.name ?: "?"),
            "usage_kind" to (usage.kind?.name ?: "?"), "usage_delta" to (usage.delta?.toString() ?: "?"))
    }

    private fun execute(operation: String, a: Map<String, String>): Map<String, String> = when (operation) {
        "ENUM" -> {
            val value = decode(a.getValue("value_hex"))
            when (a.getValue("kind")) {
                "STATE" -> TripState.parse(value).let { mapOf("value" to it.name, "terminal" to it.terminal.toString()) }
                "COMMAND" -> mapOf("value" to TripCommand.parse(value).name)
                "REVIEW" -> mapOf("value" to TripReviewState.parse(value).name)
                else -> error("Unknown fixture enum namespace")
            }
        }
        "TRANSITION" -> {
            val state = TripState.parse(a.getValue("state"))
            mapOf("state" to TripLifecycle.transition(state, TripCommand.parse(a.getValue("command"))).name)
        }
        "STANDING" -> TripStanding(TripState.parse(a.getValue("operational")), TripReviewState.parse(a.getValue("review"))).let {
            mapOf("operational" to it.operational.name, "review" to it.review.name, "completed" to it.completed.toString(),
                "approved" to it.approved.toString())
        }
        "START" -> tripFields(start(startFields(a)))
        "FINISH" -> tripFields(finish(a))
        else -> error("Unknown fixture operation")
    }

    private fun failure(error: Throwable): Map<String, String> = when (error) {
        is TripDomainException -> if (error.failure == TripFailure.INCOMPLETE) {
            mapOf("error" to "INCOMPLETE", "missing" to error.missing.joinToString(",") { it.name })
        } else mapOf("error" to error.failure.name)
        is MeterDomainException -> mapOf("error" to error.failure.name)
        is FleetDomainException -> mapOf("error" to error.failure.name)
        is EnterpriseTimeException -> mapOf("error" to "INVALID")
        else -> throw error
    }

    @TestFactory fun allSharedTripVectors(): List<DynamicTest> {
        val file = File("../contracts/testdata/enterprise/trip/trip-v1.psv")
        assertEquals(corpusHash, MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        assertEquals(207, rows.size)
        require(rows.all { it.size == 4 && it[0].isNotBlank() })
        assertEquals(rows.size, rows.map { it[0] }.distinct().size)
        assertEquals(wantCounts, rows.groupingBy { it[1] }.eachCount())
        val codes = mutableSetOf<String>()
        val pairs = mutableSetOf<String>()
        for (row in rows) {
            val args = fields(row[2])
            caseShape(row[1], args)
            val expected = fields(row[3])
            expected["error"]?.let { code ->
                require(expected.size == 1 || code == "INCOMPLETE" && expected.size == 2 && "missing" in expected)
                codes += code
            }
            if (row[1] == "TRANSITION") {
                val state = TripState.entries.firstOrNull { it.name == args["state"] }
                val command = TripCommand.entries.firstOrNull { it.name == args["command"] }
                if (state != null && command != null) pairs += "$state:$command"
            }
        }
        assertEquals(errorCodes, codes, "every error code needs a shared vector and no other code may appear")
        assertEquals(35, pairs.size, "the state and command table must be complete")
        return rows.map { (id, operation, rawArgs, expected) ->
            DynamicTest.dynamicTest(id) {
                val actual = try { execute(operation, fields(rawArgs)) } catch (failure: IllegalArgumentException) { failure(failure) }
                assertEquals(fields(expected), actual)
            }
        }
    }
}
