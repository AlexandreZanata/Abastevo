package com.anpfuel.domain.enterprise.fleet

import java.io.File
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory

/** This fixture catalog makes no claim about the future runtime product/category catalog. */
internal object SyntheticFleetCatalog : FleetCatalog {
    override fun category(code: String): CatalogCategory? = when (code) {
        "CAT_A", "CAT_B" -> CatalogCategory(code)
        "BAD_CATEGORY" -> CatalogCategory("OTHER_CATEGORY")
        else -> null
    }
    override fun product(code: String): CatalogProduct? = when (code) {
        "LIQUID_A", "LIQUID_B", "\uE000", "\uD800\uDC00" -> CatalogProduct(code, "L")
        "GAS_A" -> CatalogProduct(code, "M3")
        "POWER_A" -> CatalogProduct(code, "KWH")
        "BAD_UNIT" -> CatalogProduct(code, "OTHER")
        "BAD_PRODUCT" -> CatalogProduct("OTHER_PRODUCT", "L")
        else -> null
    }
}

class EnterpriseFleetVectorsTest {
    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map { item ->
            val pair = item.split('=', limit = 2)
            require(pair.size == 2 && pair[0].isNotEmpty())
            pair[0] to pair[1]
        }
        require(pairs.map { it.first }.toSet().size == pairs.size)
        return pairs.toMap()
    }

    private fun decode(raw: String): String {
        require(raw.length % 2 == 0 && raw.all { it in '0'..'9' || it in 'a'..'f' })
        val bytes = raw.chunked(2).map { it.toInt(16).toByte() }.toByteArray()
        return Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT)
            .onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(bytes)).toString()
    }

    private fun hex(raw: String): String = raw.toByteArray(Charsets.UTF_8).joinToString("") { "%02x".format(it.toInt() and 255) }

    private fun alias(raw: String): String = when (raw) {
        "A" -> "7b0e2a54-0000-4000-8000-000000000001"
        "B" -> "7b0e2a54-0000-4000-8000-000000000002"
        "X" -> "7b0e2a54-0000-4000-8000-000000000011"
        "Y" -> "7b0e2a54-0000-4000-8000-000000000012"
        "U" -> "7b0e2a54-0000-4000-8000-000000000021"
        "M" -> "7b0e2a54-0000-4000-8000-000000000031"
        "?" -> ""
        else -> raw
    }

    private fun tanks(raw: String): List<Tank> = if (raw.isEmpty()) emptyList() else raw.split('~').map {
        val values = it.split(':')
        require(values.size == 3)
        Tank.create(TankRole.parse(values[0]), values[1], if (values[2].isEmpty()) emptyList() else values[2].split(','), SyntheticFleetCatalog)
    }

    private fun references(raw: String): List<ConsumptionReference> = if (raw.isEmpty()) emptyList() else raw.split('~').map {
        val values = it.split(':')
        require(values.size == 5)
        ConsumptionReference.create(values[0], UsageBasis.parse(values[1]), values[2], decode(values[3]), values[4], SyntheticFleetCatalog)
    }

    private fun profile(args: Map<String, String>): AssetProfile = AssetProfile(
        reference = AssetRef(alias("A"), AssetId.parse(alias("X"))),
        kind = AssetKind.parse(args.getValue("kind")),
        categoryCode = args.getValue("category"),
        name = decode(args.getValue("name_hex")),
        fleetNumber = decode(args.getValue("fleet_hex")),
        plate = decode(args.getValue("plate_hex")),
        serial = decode(args.getValue("serial_hex")),
        basis = UsageBasis.parse(args.getValue("basis")),
        tanks = tanks(args.getValue("tanks")),
        references = references(args.getValue("references")),
        catalog = SyntheticFleetCatalog,
    )

    private fun execute(operation: String, args: Map<String, String>): Map<String, String> = when (operation) {
        "NORMALIZE" -> {
            val value = decode(args.getValue("value_hex"))
            val key = when (args.getValue("type")) {
                "PLATE" -> FleetIdentifiers.plate(value)
                "FLEET" -> FleetIdentifiers.fleetNumber(value)
                else -> throw FleetDomainException()
            }
            mapOf("key_hex" to hex(key))
        }
        "TANK" -> Tank.create(
            TankRole.parse(args.getValue("role")), args.getValue("capacity"),
            args.getValue("products").takeUnless { it.isEmpty() }?.split(',') ?: emptyList(), SyntheticFleetCatalog,
        ).let { mapOf("role" to it.role.name, "capacity" to it.capacityMilli.toString(), "unit" to it.unit, "products" to it.products.joinToString(",")) }
        "EXPECTED" -> ConsumptionReference.create(
            args.getValue("product"), UsageBasis.parse(args.getValue("basis")), args.getValue("value"),
            decode(args.getValue("source_hex")), args.getValue("date"), SyntheticFleetCatalog,
        ).let {
            mapOf("product" to it.productCode, "basis" to it.basis.name, "value" to it.value.toString(), "unit" to it.unit,
                "source_hex" to hex(it.source), "date" to it.date)
        }
        "VALIDATE" -> profile(args).let {
            mapOf("kind" to it.kind.name, "category" to it.categoryCode, "name_hex" to hex(it.name),
                "fleet_key_hex" to hex(it.fleetKey), "plate_key_hex" to hex(it.plateKey), "serial_hex" to hex(it.serial),
                "basis" to it.basis.name, "tanks" to it.tanks.size.toString(), "references" to it.consumptionReferences.size.toString())
        }
        "ACTIVATE" -> {
            val value = profile(args)
            val unit = if (args.getValue("unit_org") == "?" && args.getValue("unit_id") == "?") null
                else UnitFact(alias(args.getValue("unit_org")), alias(args.getValue("unit_id")))
            val meters = args.getValue("meters").takeUnless { it.isEmpty() }?.split('~')?.map {
                val values = it.split(','); require(values.size == 4)
                MeterFact(AssetRef(alias(values[0]), AssetId.parse(alias(values[1]))), alias(values[2]), values[3].toBooleanStrict())
            } ?: emptyList()
            AssetRules.activation(value, unit, meters).let {
                if (it.ready) mapOf("ready" to "true") else mapOf("error" to "INCOMPLETE", "missing" to it.missing.joinToString(",") { reason -> reason.name })
            }
        }
        "CHANGE_KIND" -> mapOf("kind" to AssetRules.kindAfterChange(
            AssetKind.parse(args.getValue("from")), AssetKind.parse(args.getValue("to")), args.getValue("accepted").toBooleanStrict(),
        ).name)
        "STATE" -> mapOf("state" to AssetLifecycle.parse(args.getValue("value")).name)
        "SCOPE" -> {
            AssetRef(alias(args.getValue("org")), AssetId.parse(alias(args.getValue("asset"))))
                .requireOrganization(alias(args.getValue("request_org")))
            mapOf("valid" to "true")
        }
        else -> error("Unhandled shared fleet operation")
    }

    @TestFactory fun allSharedAssetVectors(): List<DynamicTest> {
        val lines = File("../contracts/testdata/enterprise/fleet/asset-v1.psv").readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        assertEquals(133, rows.size)
        require(rows.all { it.size == 4 && it[0].isNotEmpty() })
        assertEquals(rows.size, rows.map { it[0] }.toSet().size)
        assertEquals(mapOf("NORMALIZE" to 23, "TANK" to 24, "EXPECTED" to 27, "VALIDATE" to 23,
            "ACTIVATE" to 16, "CHANGE_KIND" to 8, "STATE" to 6, "SCOPE" to 6), rows.groupingBy { it[1] }.eachCount())
        return rows.map { (id, operation, rawArgs, rawExpected) -> DynamicTest.dynamicTest(id) {
            val result = try { execute(operation, fields(rawArgs)) }
            catch (failure: FleetDomainException) { mapOf("error" to failure.failure.name) }
            assertEquals(fields(rawExpected), result)
        } }
    }
}
