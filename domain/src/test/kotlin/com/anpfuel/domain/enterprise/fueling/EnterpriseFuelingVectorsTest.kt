package com.anpfuel.domain.enterprise.fueling

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetId
import com.anpfuel.domain.enterprise.fleet.AssetRef
import com.anpfuel.domain.enterprise.fleet.FleetDomainException
import com.anpfuel.domain.enterprise.fleet.FleetFailure
import com.anpfuel.domain.enterprise.meter.*
import java.io.File
import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** Independent shared expectations; no amount, workflow, issuer or unit policy is implemented here. */
class EnterpriseFuelingVectorsTest {
    private val aliases = mapOf("A" to 1, "B" to 2, "X" to 11, "Y" to 12, "E" to 21, "J" to 22, "U" to 31, "V" to 32,
        "M" to 41, "N" to 42, "S" to 51, "T" to 52, "R" to 61, "W" to 62, "K" to 71, "G" to 72, "H" to 81,
        "P" to 91, "Q" to 92, "Z" to 111, "C" to 112, "L" to 121, "D" to 122, "F" to 131)
    private fun id(index: Int) = "9d000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private fun alias(raw: String): String = if (raw == "?") "" else aliases[raw]?.let(::id) ?: raw
    private fun label(raw: String): String = aliases.entries.firstOrNull { id(it.value) == raw }?.key ?: raw
    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotEmpty()); p[0] to p[1] }
        require(pairs.map { it.first }.distinct().size == pairs.size)
        return pairs.toMap()
    }
    private fun hex(raw: String) = raw.toByteArray(Charsets.UTF_8).joinToString("") { "%02x".format(it.toInt() and 255) }
    private fun decode(raw: String): String {
        require(raw.length % 2 == 0 && raw.all { it in '0'..'9' || it in 'a'..'f' })
        return Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT)
            .decode(ByteBuffer.wrap(raw.chunked(2).map { it.toInt(16).toByte() }.toByteArray())).toString()
    }
    private val operations = setOf("CATALOG", "PRODUCT", "ENUM", "QUANTITY", "CANDIDATE", "ROLES", "TOTAL")
    private val enumSelectors = setOf("FILL", "PAYER", "PAYMENT", "ORIGIN", "PLACE")
    private fun caseShape(operation: String, args: Map<String, String>) {
        require(operation in operations)
        if (operation == "ENUM") require(args.getValue("kind") in enumSelectors)
    }
    @Test fun `unknown fixture namespaces are rejected before domain dispatch`() {
        assertThrows(IllegalArgumentException::class.java) { caseShape("ENUM", fields("kind=OTHER;value_hex=46554c4c")) }
        assertThrows(IllegalArgumentException::class.java) { caseShape("OTHER", fields("all=true")) }
        assertThrows(CharacterCodingException::class.java) { decode("ff") }
        assertThrows(CharacterCodingException::class.java) { decode("eda080") }
    }
    private fun asset(org: String, asset: String) = AssetRef(alias(org), AssetId.parse(alias(asset)))
    private fun optional(a: Map<String, String>, key: String): String? = a[key]?.takeUnless { it == "?" }
    private fun candidate(a: Map<String, String>): FuelingCandidate {
        val omitted = a["omit"]?.split(',').orEmpty().toSet()
        val owner = a["org"] ?: "A"
        val asset = if ("asset" in omitted) null else asset(a["asset_org"] ?: owner, a["asset"] ?: "X")
        val reading = if ("reading" in omitted) null else when (a["reading"] ?: "REFERENCE") {
            "?" -> null
            "MISSING" -> SupplyReading.Missing(MeterPresence.missing(a["missing_hex"]?.let(::decode) ?: "Display unavailable"))
            "REFERENCE" -> {
                val org = a["reading_org"] ?: owner
                val refAsset = asset(org, a["reading_asset"] ?: a["asset"] ?: "X")
                val meter = MeterRef(alias(org), refAsset, alias(a["reading_meter"] ?: "M"), MeterKind.parse(a["reading_kind"] ?: "ODOMETER"))
                SupplyReading.Reference(ReadingRef(alias(org), SegmentRef(alias(org), meter, alias(a["reading_segment"] ?: "S")), alias(a["reading_id"] ?: "R")))
            }
            else -> throw FuelDomainException()
        }
        val place = optional(a, "place")?.let { when (PlaceKind.parse(it)) {
            PlaceKind.CATALOG_STATION -> Establishment.Station(PublicStationRef(alias(a["place_ref"] ?: "?")))
            PlaceKind.ORG_SUPPLIER -> Establishment.Supplier(SupplierRef(alias(a["place_org"] ?: owner), alias(a["place_ref"] ?: "?")))
            PlaceKind.INTERNAL_TANK -> Establishment.InternalTank(InternalSourceTankRef(alias(a["place_org"] ?: owner), alias(a["place_ref"] ?: "?")))
            PlaceKind.NOT_FOUND -> Establishment.NotFound(a["place_name_hex"]?.let(::decode) ?: "")
        } }
        return FuelingCandidate.create(CandidateInput(
            reference = if ("id" in omitted) null else FuelingRef(alias(owner), alias(a["id"] ?: "F")),
            asset = asset,
            responsible = if ("responsible" in omitted) null else EmployeeRef(alias(a["responsible_org"] ?: owner), alias(a["responsible"] ?: "E")),
            author = if ("author" in omitted) null else MembershipRef(alias(a["author_org"] ?: owner), alias(a["author"] ?: "U")),
            effective = if ("effective" in omitted) null else TimeDeclaration(EnterpriseTime.parseInstant(a["effective"] ?: "2026-11-14T12:00:00Z"),
                a["entry"] ?: "UTC", (a["offset"] ?: "0").toLongOrNull() ?: throw FuelDomainException()),
            quantity = if ("product" in omitted || "quantity" in omitted) null else ProductQuantity.parse(a["product"] ?: "ETHANOL", a["unit"] ?: "L", a["quantity"] ?: "150000"),
            amounts = if ("paid" in omitted) null else RecordedAmounts.parse(a["paid"] ?: "97350", optional(a, "discount"), optional(a, "unit_price"),
                optional(a, "price_text_hex")?.let(::decode)),
            fill = if ("fill" in omitted) null else FillLevel.parse(a["fill"] ?: "PARTIAL"),
            payer = if ("payer" in omitted) null else Payer.parse(a["payer"] ?: "COMPANY"),
            payment = if ("payment" in omitted) null else PaymentMethod.parse(a["payment"] ?: "CASH"),
            reading = reading,
            origin = (a["origin"] ?: "APP").takeUnless { it == "?" }?.let(SupplyOrigin::parse),
            tank = optional(a, "tank")?.let { DestinationTankRef(asset(a["tank_org"] ?: owner, a["tank_asset"] ?: a["asset"] ?: "X"), alias(it)) },
            group = optional(a, "group")?.let { DestinationGroupRef(asset(a["group_org"] ?: owner, a["group_asset"] ?: a["asset"] ?: "X"), alias(it)) },
            trip = optional(a, "trip")?.let { TripRef(alias(a["trip_org"] ?: owner), alias(it)) },
            establishment = place,
            contractedSupplier = optional(a, "contracted")?.let { SupplierRef(alias(a["contracted_org"] ?: owner), alias(it)) },
        ))
    }
    private fun place(candidate: FuelingCandidate): Map<String, String> {
        val value = candidate.establishment
        val ref = when (value) {
            is Establishment.Station -> value.station.id
            is Establishment.Supplier -> value.supplier.id
            is Establishment.InternalTank -> value.source.id
            is Establishment.NotFound, null -> null
        }
        return mapOf("place" to (value?.kind?.name ?: "?"), "place_ref" to (ref?.let(::label) ?: "?"),
            "place_name_hex" to ((value as? Establishment.NotFound)?.name?.let(::hex) ?: ""), "contracted" to (candidate.contractedSupplier?.id?.let(::label) ?: "?"))
    }
    private fun execute(operation: String, a: Map<String, String>): Map<String, String> = when (operation) {
        "CATALOG" -> {
            require(a.getValue("all") == "true")
            mapOf("version" to FuelProducts.VERSION, "count" to FuelProducts.all().size.toString(), "keys" to FuelProducts.all().map { it.name }.sorted().joinToString(","))
        }
        "PRODUCT" -> FuelProducts.get(a.getValue("key")).let { mapOf("version" to FuelProducts.VERSION, "key" to it.name, "category" to it.category.name,
            "unit" to it.unit.name, "storage" to it.storage.name, "family" to it.family.name, "additive" to it.isAdditive.toString()) }
        "ENUM" -> {
            val value = decode(a.getValue("value_hex"))
            when (a.getValue("kind")) {
                "FILL" -> FillLevel.parse(value).let { mapOf("value" to it.name, "effective" to it.effective.name) }
                "PAYER" -> mapOf("value" to Payer.parse(value).name)
                "PAYMENT" -> mapOf("value" to PaymentMethod.parse(value).name)
                "ORIGIN" -> mapOf("value" to SupplyOrigin.parse(value).name)
                "PLACE" -> mapOf("value" to PlaceKind.parse(value).name)
                else -> error("Unknown fixture enum namespace")
            }
        }
        "QUANTITY" -> ProductQuantity.parse(a.getValue("product"), a.getValue("unit"), a.getValue("amount")).let {
            mapOf("product" to it.product.name, "category" to it.product.category.name, "unit" to it.unit.name, "amount" to it.amount.toString()) }
        "CANDIDATE" -> {
            val value = candidate(a)
            val reading = value.reading
            mapOf("ready" to "true", "org" to label(value.reference.organizationId), "id" to label(value.reference.id), "asset" to label(value.asset.assetId.value),
                "responsible" to label(value.responsible.id), "author" to label(value.author.id), "product" to value.quantity.product.name, "unit" to value.quantity.unit.name,
                "quantity" to value.quantity.amount.toString(), "paid" to value.amounts.paid.toString(), "discount" to (value.amounts.discount?.toString() ?: "?"),
                "unit_price" to (value.amounts.unitPrice?.toString() ?: "?"), "price_text_hex" to (value.amounts.sourcePriceText?.let(::hex) ?: "?"),
                "fill" to value.fill.name, "effective_fill" to value.fill.effective.name, "payer" to value.payer.name, "payment" to value.payment.name,
                "reading" to (if (reading is SupplyReading.Reference) "REFERENCE" else "MISSING"),
                "reading_id" to ((reading as? SupplyReading.Reference)?.reference?.id?.let(::label) ?: "?"),
                "missing_hex" to ((reading as? SupplyReading.Missing)?.missing?.reason?.let(::hex) ?: ""),
                "tank" to (value.tank?.id?.let(::label) ?: "?"), "group" to (value.group?.id?.let(::label) ?: "?"), "trip" to (value.trip?.id?.let(::label) ?: "?"),
                "origin" to (value.origin?.name ?: "?"), "effective" to EnterpriseTime.formatInstant(value.effective.occurredAt), "entry" to value.effective.entryZone,
                "offset" to value.effective.deviceOffsetSeconds.toString(), "additive" to value.quantity.product.isAdditive.toString()) + place(value)
        }
        "ROLES" -> {
            val value = candidate(a)
            val issuer = optional(a, "doc")?.let { DocumentIssuerProjection(DocumentRef(alias(a["doc_org"] ?: a["org"] ?: "A"), alias(it)),
                (a["doc_revision"] ?: "1").toLongOrNull() ?: throw FuelDomainException(), a["issuer_hex"]?.let(::decode) ?: "Synthetic document issuer") }
            val roles = FuelingRolesProjection(value, issuer)
            place(roles.candidate) + mapOf("doc" to (roles.issuer?.document?.id?.let(::label) ?: "?"),
                "doc_revision" to (roles.issuer?.sourceRevision?.toString() ?: "?"), "issuer_hex" to (roles.issuer?.issuerLabel?.let(::hex) ?: "?"))
        }
        "TOTAL" -> {
            val inputs = a.getValue("items").takeIf { it.isNotEmpty() }?.split('~')?.map {
                val tuple = it.split(','); require(tuple.size == 3); ProductQuantity.parse(tuple[0], tuple[1], tuple[2])
            }.orEmpty()
            when (a.getValue("mode")) {
                "PRODUCT" -> PurchasedQuantities.byProduct(inputs).let { groups -> mapOf("count" to groups.size.toString(),
                    "groups" to groups.joinToString("~") { "${it.product.name}:${it.product.category.name}:${it.product.unit.name}:${it.amount}" }) }
                "COMPATIBLE" -> PurchasedQuantities.compatible(inputs).let { mapOf("category" to it.category.name, "unit" to it.unit.name, "amount" to it.amount.toString()) }
                else -> error("Unknown fixture total namespace")
            }
        }
        else -> error("Unknown fixture operation")
    }
    @TestFactory fun allSharedFuelingVectors(): List<DynamicTest> {
        val file = File("../contracts/testdata/enterprise/fueling/fueling-v1.psv")
        assertEquals("bca4934b4a21935cd1a1728c087fadef5d0935fa39f596b261f844464685395a",
            MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        assertEquals(200, rows.size)
        require(rows.all { it.size == 4 && it[0].isNotEmpty() })
        assertEquals(rows.size, rows.map { it[0] }.distinct().size)
        assertEquals(mapOf("CATALOG" to 1, "PRODUCT" to 15, "ENUM" to 38, "QUANTITY" to 27, "CANDIDATE" to 91, "ROLES" to 15, "TOTAL" to 13), rows.groupingBy { it[1] }.eachCount())
        for (row in rows) caseShape(row[1], fields(row[2]))
        return rows.map { (id, operation, rawArgs, expected) -> DynamicTest.dynamicTest(id) {
            val actual = try { execute(operation, fields(rawArgs)) }
            catch (failure: FuelDomainException) { mapOf("error" to failure.failure.name) }
            catch (_: EnterpriseTimeException) { mapOf("error" to "INVALID") }
            catch (failure: MeterDomainException) { mapOf("error" to if (failure.failure == MeterFailure.SCOPE) "SCOPE" else "INVALID") }
            catch (failure: FleetDomainException) { mapOf("error" to if (failure.failure == FleetFailure.SCOPE) "SCOPE" else "INVALID") }
            assertEquals(fields(expected), actual)
        } }
    }
}
