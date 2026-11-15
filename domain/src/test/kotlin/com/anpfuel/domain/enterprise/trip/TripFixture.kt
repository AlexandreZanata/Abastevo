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
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertEquals

/** The fixture grammar shared by the trip-v1 and trip-time-v1 consumers (same rules as the Go harness). */
internal object TripFixture {
    private val aliases = mapOf("A" to 1, "B" to 2, "T" to 11, "T2" to 12, "V" to 21, "V2" to 22, "D" to 31, "D2" to 32,
        "M" to 41, "M2" to 42, "MT" to 51, "MT2" to 52, "SG" to 61, "SG2" to 62, "R1" to 71, "R2" to 72, "R3" to 73)
    val startOmittable = setOf("id", "author", "asset", "driver", "departure", "declaration", "zone")
    private val finishOmittable = setOf("author", "final", "declaration")
    private val departureReasonHex = hex("Odometro ilegivel")
    private val finalReasonHex = hex("Sem leitura final")

    fun hex(raw: String): String = raw.toByteArray(Charsets.UTF_8).joinToString("") { "%02x".format(it.toInt() and 255) }
    private fun id(index: Int) = "5e000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    fun alias(raw: String): String = aliases[raw]?.let(::id) ?: raw
    fun label(raw: String): String = aliases.entries.firstOrNull { id(it.value) == raw }?.key ?: raw
    fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotBlank()); p[0] to p[1] }
        require(pairs.map { it.first }.distinct().size == pairs.size)
        return pairs.toMap()
    }
    fun decode(raw: String): String {
        require(raw.length % 2 == 0 && raw.all { it in '0'..'9' || it in 'a'..'f' })
        return Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT)
            .decode(ByteBuffer.wrap(raw.chunked(2).map { it.toInt(16).toByte() }.toByteArray())).toString()
    }
    fun present(raw: String?): String? = raw?.takeUnless { it == "?" }
    fun omitted(raw: String?, allowed: Set<String>): Set<String> {
        if (raw.isNullOrEmpty()) return emptySet()
        val keys = raw.split(',')
        require(keys.all { it in allowed } && keys.distinct().size == keys.size)
        return keys.toSet()
    }

    /** Reads a corpus after asserting its exact hash and header; rows are id, operation, args, expected. */
    fun corpus(name: String, hash: String): List<List<String>> {
        val file = File("../contracts/testdata/enterprise/trip/$name")
        assertEquals(hash, MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        require(rows.all { it.size == 4 && it[0].isNotBlank() })
        assertEquals(rows.size, rows.map { it[0] }.distinct().size)
        return rows
    }

    /** Decodes a fixture run clock; malformed fixture syntax fails the loader. */
    fun clock(bootHex: String, raw: String): TripRunClock = TripRunClock(decode(bootHex), requireNotNull(raw.toLongOrNull()))

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

    fun startFields(args: Map<String, String>): Map<String, String> {
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

    fun start(f: Map<String, String>): Trip {
        val omit = omitted(f["omit"], startOmittable)
        val reference = if ("id" in omit) null else TripRef(alias(f.getValue("id_org")), alias(f.getValue("id")))
        val author = if ("author" in omit) null else MembershipRef(alias(f.getValue("author_org")), alias(f.getValue("author")))
        val asset = if ("asset" in omit) null else AssetId.parse(alias(f.getValue("asset"))).let { AssetRef(alias(f.getValue("asset_org")), it) }
        val driver = if ("driver" in omit) null else EmployeeRef(alias(f.getValue("driver_org")), alias(f.getValue("driver")))
        val departure = if ("departure" in omit) null else evidence(f, "dep")
        val declared = if ("declaration" in omit) null else declaration(f.getValue("at"), f.getValue("entry"), f.getValue("offset"))
        val zones = if ("zone" in omit) null else ReportingZones(f.getValue("zone"), present(f["unit_zone"]))
        val runClock = f["boot_hex"]?.let { clock(it, f.getValue("mono")) }
        return Trip.start(alias(f.getValue("scope")), TripStartInput(reference, author, asset, driver, departure, declared, zones, runClock))
    }

    fun finish(args: Map<String, String>): Trip {
        val f = finishFields(startFields(args))
        val started = start(f)
        val input = finishInput(f)
        val again = f["again"] ?: "false"
        require(again == "true" || again == "false")
        val before = tripFields(started)
        val scope = alias(f.getValue("fin_scope"))
        val next = try { started.finish(scope, input) } finally { assertEquals(before, tripFields(started), "finish changed its input trip") }
        return if (again == "true") next.finish(scope, input) else next
    }

    private fun finishFields(start: Map<String, String>): MutableMap<String, String> {
        val f = start.toMutableMap()
        val defaults = mapOf("fin_scope" to f.getValue("scope"), "fin_author" to "M", "fin_author_org" to f.getValue("org"),
            "fin" to "READING", "fin_org" to f.getValue("org"), "fin_asset" to f.getValue("asset"), "fin_meter" to f.getValue("dep_meter"),
            "fin_segment" to f.getValue("dep_segment"), "fin_reading" to "R2", "fin_kind" to f.getValue("dep_kind"),
            "fin_value" to "184610000", "fin_modulus" to f.getValue("dep_modulus"), "fin_reason_hex" to finalReasonHex,
            "fin_at" to "2026-11-14T15:00:00Z", "fin_entry" to "America/Cuiaba", "fin_offset" to "-14400")
        defaults.forEach { (k, v) -> f.putIfAbsent(k, v) }
        f.putIfAbsent("fin_value_kind", f.getValue("fin_kind"))
        return f
    }

    private fun finishInput(f: Map<String, String>): TripFinishInput {
        val omit = omitted(f["fin_omit"], finishOmittable)
        val author = if ("author" in omit) null else MembershipRef(alias(f.getValue("fin_author_org")), alias(f.getValue("fin_author")))
        val final = if ("final" in omit) null else evidence(f, "fin")
        val declared = if ("declaration" in omit) null else declaration(f.getValue("fin_at"), f.getValue("fin_entry"), f.getValue("fin_offset"))
        val runClock = f["fin_boot_hex"]?.let { clock(it, f.getValue("fin_mono")) }
        return TripFinishInput(author, final, declared, runClock)
    }

    private val commandOmittable = setOf("author", "declaration")

    private fun apply(trip: Trip, command: TripCommand, scope: String, input: TripLifecycleInput): Trip = when (command) {
        TripCommand.PAUSE -> trip.pause(scope, input)
        TripCommand.RESUME -> trip.resume(scope, input)
        TripCommand.CANCEL -> trip.cancel(scope, input)
        else -> error("Not a lifecycle fixture command")
    }

    /** Applies the trip-v2 fixture history: step i at 12:i0Z by M of the trip organization; FINISH with its v1 defaults. */
    private fun withHistory(started: Trip, f: Map<String, String>): Trip {
        val raw = f["history"] ?: return started
        val steps = raw.split(',')
        require(steps.size <= 9)
        val scope = alias(f.getValue("scope"))
        var trip = started
        steps.forEachIndexed { i, step ->
            trip = if (step == "FINISH") trip.finish(scope, finishInput(finishFields(f))) else {
                val command = TripCommand.parse(step)
                require(command == TripCommand.PAUSE || command == TripCommand.RESUME || command == TripCommand.CANCEL)
                val input = TripLifecycleInput(MembershipRef(alias(f.getValue("org")), alias("M")), trip.revision,
                    if (command == TripCommand.CANCEL) "Cliente cancelou" else null,
                    declaration("2026-11-14T12:${i + 1}0:00Z", "America/Cuiaba", "-14400"))
                apply(trip, command, scope, input)
            }
        }
        return trip
    }

    fun history(args: Map<String, String>): Map<String, String> {
        val f = startFields(args)
        val trip = withHistory(start(f), f)
        val steps = trip.lifecycle.map { "${it.command.name}/${it.revision}" } + listOfNotNull(trip.finished?.let { "FINISH/${trip.revision}" })
        return tripFields(trip) + mapOf("revision" to trip.revision.toString(), "steps" to (steps.joinToString(",").ifEmpty { "?" }))
    }

    fun command(command: TripCommand, args: Map<String, String>): Map<String, String> {
        val f = startFields(args).toMutableMap()
        val trip = withHistory(start(f), f)
        val defaults = mapOf("cmd_scope" to f.getValue("scope"), "by" to "M", "by_org" to f.getValue("org"), "expected" to trip.revision.toString(),
            "reason_hex" to if (command == TripCommand.CANCEL) hex("Cliente cancelou") else "?", "cmd_at" to "2026-11-14T14:00:00Z",
            "cmd_entry" to "America/Cuiaba", "cmd_offset" to "-14400")
        defaults.forEach { (k, v) -> f.putIfAbsent(k, v) }
        val omit = omitted(f["cmd_omit"], commandOmittable)
        val expected = requireNotNull(f.getValue("expected").toLongOrNull())
        val author = if ("author" in omit) null else MembershipRef(alias(f.getValue("by_org")), alias(f.getValue("by")))
        val reason = present(f["reason_hex"])?.let(::decode)
        val declared = if ("declaration" in omit) null else declaration(f.getValue("cmd_at"), f.getValue("cmd_entry"), f.getValue("cmd_offset"))
        val input = TripLifecycleInput(author, expected, reason, declared)
        val scope = alias(f.getValue("cmd_scope"))
        val again = f["again"] ?: "false"
        require(again == "true" || again == "false")
        val before = tripFields(trip)
        var next = try { apply(trip, command, scope, input) } finally { assertEquals(before, tripFields(trip), "a lifecycle command changed its input trip") }
        if (again == "true") next = apply(next, command, scope, input)
        val last = next.lifecycle.last()
        check(last.command == command && last.to == next.state && last.revision == next.revision)
        return tripFields(next) + mapOf("revision" to next.revision.toString(), "previous" to last.from.name, "command_author" to label(last.author.id),
            "command_at" to EnterpriseTime.formatInstant(last.declared.occurredAt), "command_entry" to last.declared.entryZone,
            "command_offset" to last.declared.deviceOffsetSeconds.toString(), "command_reason_hex" to (last.reason?.let(::hex) ?: "?"))
    }

    private fun evidenceField(evidence: TripMeterEvidence?): String = when (evidence) {
        is TripMeterEvidence.Observed -> "${label(evidence.reference.id)}:${evidence.value.kind.name}:${evidence.value.units}"
        is TripMeterEvidence.Absent -> "ABSENT:${hex(evidence.missing.reason)}"
        null -> "?"
    }

    fun tripFields(trip: Trip): Map<String, String> {
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

    /** Maps the trip vocabulary; fixture-built meter, fleet and time values keep their own INVALID/SCOPE meaning. */
    fun failure(error: Throwable): Map<String, String> = when (error) {
        is TripDomainException -> if (error.failure == TripFailure.INCOMPLETE) {
            mapOf("error" to "INCOMPLETE", "missing" to error.missing.joinToString(",") { it.name })
        } else mapOf("error" to error.failure.name)
        is MeterDomainException -> mapOf("error" to error.failure.name)
        is FleetDomainException -> mapOf("error" to error.failure.name)
        is EnterpriseTimeException -> mapOf("error" to "INVALID")
        else -> throw error
    }
}
