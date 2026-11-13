package com.anpfuel.domain.enterprise.fleet

import com.anpfuel.domain.valueobject.DomainId
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseFleetTest {
    @Test fun `identifier keys follow the literal ASCII contract without locale expansion`() {
        assertEquals("ABC1D23", FleetIdentifiers.plate("abc-1d23"))
        assertEquals("ABC1D23", FleetIdentifiers.plate("ABC1D23"))
        assertEquals("TRUCK/07", FleetIdentifiers.fleetNumber(" truck-/07 "))
        for (raw in listOf("", "a", "ABCDEFGH", "ABC/123", "abc\u00a01d23", "abc\t1d23", "abK1d23", "abc\n1d23")) {
            assertThrows(FleetDomainException::class.java) { FleetIdentifiers.plate(raw) }
        }
        for (raw in listOf("", " - ", "x".repeat(21), "frota\u00a0", "frota\u0000", "Straße")) {
            assertThrows(FleetDomainException::class.java) { FleetIdentifiers.fleetNumber(raw) }
        }
        assertEquals("!/#", FleetIdentifiers.fleetNumber(" !-/# "))
    }

    @Test fun `enterprise asset references are explicit typed and redacted`() {
        val org = "7b0e2a54-0000-4000-8000-000000000001"
        val id = AssetId.parse("7b0e2a54-0000-4000-8000-000000000002")
        val ref = AssetRef(org, id)
        assertEquals(id, ref.assetId)
        ref.requireOrganization(org)
        assertThrows(FleetDomainException::class.java) { ref.requireOrganization("7b0e2a54-0000-4000-8000-000000000003") }
        assertFalse(ref.toString().contains(org))
        assertFalse(id.toString().contains(id.value))
        val personal = DomainId.from(id.value)
        assertFalse(id.equals(personal))
        assertFalse(personal.equals(id))
        assertEquals(id, AssetId.parse(id.value))
        assertEquals(id.hashCode(), AssetId.parse(id.value).hashCode())
        assertEquals(ref.hashCode(), AssetRef(org, id).hashCode())
        assertThrows(FleetDomainException::class.java) { AssetId.parse("00000000-0000-0000-0000-000000000000") }
        assertThrows(FleetDomainException::class.java) { AssetId.parse("personal-vehicle") }
        assertThrows(FleetDomainException::class.java) { AssetId.parse(id.value.uppercase()) }
    }

    private val catalog = SyntheticFleetCatalog
    private val org = "7b0e2a54-0000-4000-8000-000000000001"
    private val ref get() = AssetRef(org, AssetId.parse("7b0e2a54-0000-4000-8000-000000000011"))
    private fun tank(products: List<String> = listOf("LIQUID_A")) = Tank.create(TankRole.MAIN, "275000", products, catalog)
    private fun expected(product: String = "LIQUID_A") = ConsumptionReference.create(product, UsageBasis.DISTANCE, "8500", "Synthetic maker", "2026-11-01", catalog)
    private fun profile(
        name: String = "Synthetic asset",
        tanks: List<Tank> = listOf(tank()),
        references: List<ConsumptionReference> = emptyList(),
        serial: String = "",
        currentCatalog: FleetCatalog = catalog,
    ) = AssetProfile(ref, AssetKind.VEHICLE, "CAT_A", name, "Fr-01", "abc-1d23", serial,
        UsageBasis.DISTANCE, tanks, references, currentCatalog)

    @Test fun `exact quantities and reference bases never gain an invented physical ceiling`() {
        assertEquals(275_000L, tank().capacityMilli)
        assertEquals(8_500L, expected().value)
        assertEquals(UsageBasis.DISTANCE, expected().basis)
        val hours = ConsumptionReference.create("LIQUID_A", UsageBasis.HOURS, "12000", "Synthetic maker", "2026-11-01", catalog)
        assertEquals(12_000L, hours.value)
        for (basis in UsageBasis.entries) assertEquals(Long.MAX_VALUE,
            ConsumptionReference.create("LIQUID_A", basis, Long.MAX_VALUE.toString(), "Synthetic maker", "9999-12-31", catalog).value)
        // Role does not invent a product-unit restriction absent from the contract.
        assertEquals("L", Tank.create(TankRole.BATTERY, "1", listOf("LIQUID_A"), catalog).unit)
    }

    @Test fun `collections keep independent snapshots at creation and at every getter`() {
        val products = mutableListOf("LIQUID_B", "LIQUID_A", "LIQUID_A")
        val t = tank(products)
        products.clear()
        assertEquals(listOf("LIQUID_A", "LIQUID_B"), t.products)
        (t.products as MutableList<String>).clear()
        assertEquals(listOf("LIQUID_A", "LIQUID_B"), t.products)
        assertTrue(t.accepts("LIQUID_A"))
        assertFalse(t.accepts("GAS_A"))
        val tanks = mutableListOf(t, tank())
        val references = mutableListOf(expected(), expected("LIQUID_B"))
        val value = profile(tanks = tanks, references = references)
        tanks.clear(); references.clear()
        (value.tanks as MutableList<Tank>).clear()
        (value.consumptionReferences as MutableList<ConsumptionReference>).clear()
        assertEquals(2, value.tanks.size)
        assertEquals(2, value.consumptionReferences.size)
        val readiness = AssetRules.activation(profile(tanks = emptyList()), null, emptyList())
        (readiness.missing as MutableList<ActivationMissing>).clear()
        assertEquals(listOf(ActivationMissing.UNIT, ActivationMissing.TANK, ActivationMissing.METER), readiness.missing)
        assertFalse(readiness.ready)
    }

    @Test fun `profile checks descriptors again against its explicit catalog snapshot`() {
        val capturedTank = tank()
        val capturedReference = expected()
        val changedCatalog = object : FleetCatalog {
            override fun category(code: String) = CatalogCategory(code)
            override fun product(code: String) = CatalogProduct(code, "M3")
        }
        assertThrows(FleetDomainException::class.java) { profile(tanks = listOf(capturedTank), currentCatalog = changedCatalog) }
        val newTank = Tank.create(TankRole.MAIN, "1", listOf("LIQUID_A"), changedCatalog)
        assertThrows(FleetDomainException::class.java) { profile(tanks = listOf(newTank), references = listOf(capturedReference), currentCatalog = changedCatalog) }
        assertThrows(FleetDomainException::class.java) { profile(references = listOf(expected(), expected())) }
        assertThrows(FleetDomainException::class.java) { profile(references = listOf(expected("GAS_A"))) }
    }

    @Test fun `metadata is bounded by Unicode scalar count and preserves raw purpose data`() {
        assertEquals("🚚".repeat(80), profile(name = "🚚".repeat(80)).name)
        assertEquals("  serial  ", profile(serial = "  serial  ").serial)
        assertEquals("   ", profile(serial = "   ").serial)
        assertEquals("Fr-01", profile().fleetNumber)
        assertEquals("abc-1d23", profile().plate)
        for (raw in listOf("\uD800", "\uDC00", "\uD800X", "\u0085", " ", "🚚".repeat(81))) {
            assertThrows(FleetDomainException::class.java) { profile(name = raw) }
        }
        assertThrows(FleetDomainException::class.java) { profile(serial = "\uD800") }
        assertThrows(FleetDomainException::class.java) { profile(serial = "x".repeat(41)) }
        assertThrows(FleetDomainException::class.java) { ConsumptionReference.create("LIQUID_A", UsageBasis.HOURS, "1", "\uD800", "2026-11-01", catalog) }
    }

    @Test fun `current activation uses explicit same-asset in-service facts and redacts diagnostics`() {
        val value = profile(references = listOf(expected()))
        val unit = UnitFact(org, "7b0e2a54-0000-4000-8000-000000000021")
        val meter = MeterFact(ref, "7b0e2a54-0000-4000-8000-000000000031", true)
        assertTrue(AssetRules.activation(value, unit, listOf(meter)).ready)
        assertEquals(org, unit.organizationId)
        assertTrue(unit.unitId.endsWith("021"))
        assertTrue(meter.meterId.endsWith("031"))
        val other = MeterFact(AssetRef(org, AssetId.parse("7b0e2a54-0000-4000-8000-000000000012")), meter.meterId, true)
        assertEquals(FleetFailure.SCOPE, assertThrows(FleetDomainException::class.java) { AssetRules.activation(value, unit, listOf(other)) }.failure)
        for (privateValue in listOf(value, ref, ref.assetId, unit, meter, tank(), expected(), CatalogCategory("Synthetic private category"), CatalogProduct("Synthetic private product", "L"))) {
            assertFalse(privateValue.toString().contains("Synthetic"))
            assertFalse(privateValue.toString().contains(org))
            assertFalse(privateValue.toString().contains("275000"))
        }
    }

    @Test fun `optional draft fields stay optional without silently satisfying activation`() {
        val implement = AssetProfile(ref, AssetKind.IMPLEMENT, "CAT_A", "Synthetic implement",
            serial = "SN-01", basis = UsageBasis.DISTANCE, catalog = catalog)
        assertEquals("", implement.fleetNumber)
        assertEquals("", implement.plate)
        assertTrue(implement.tanks.isEmpty())
        assertTrue(implement.consumptionReferences.isEmpty())
        assertEquals(listOf(ActivationMissing.UNIT), AssetRules.activation(implement, null, emptyList()).missing)
        val vehicle = AssetProfile(ref, AssetKind.VEHICLE, "CAT_A", "Synthetic vehicle",
            fleetNumber = "V-01", basis = UsageBasis.DISTANCE, catalog = catalog)
        assertEquals(listOf(ActivationMissing.UNIT, ActivationMissing.TANK, ActivationMissing.METER),
            AssetRules.activation(vehicle, null, emptyList()).missing)
        assertThrows(FleetDomainException::class.java) {
            AssetProfile(ref, AssetKind.MACHINE, "CAT_A", "Synthetic machine", basis = UsageBasis.HOURS, catalog = catalog)
        }
        val sameIdAnotherOrganization = AssetRef("7b0e2a54-0000-4000-8000-000000000002", ref.assetId)
        assertNotEquals(ref, sameIdAnotherOrganization)
        assertFalse(ref.equals(null))
    }

    @Test fun `catalog text is well formed and product ordering matches Unicode scalar ordering`() {
        val explicitCatalog = object : FleetCatalog {
            override fun category(code: String) = CatalogCategory(code)
            override fun product(code: String) = CatalogProduct(code, "L")
        }
        assertEquals(listOf("P", "PX", "\uE000", "\uD800\uDC00"),
            Tank.create(TankRole.MAIN, "1", listOf("\uD800\uDC00", "PX", "\uE000", "P"), explicitCatalog).products)
        for (code in listOf("", " ", "\u0085", "\uD800", "\uDC00")) {
            assertThrows(FleetDomainException::class.java) { Tank.create(TankRole.MAIN, "1", listOf(code), explicitCatalog) }
            assertThrows(FleetDomainException::class.java) {
                AssetProfile(ref, AssetKind.IMPLEMENT, code, "Synthetic implement", serial = "SN-01",
                    basis = UsageBasis.HOURS, catalog = explicitCatalog)
            }
        }
        assertEquals("🚚".repeat(280), ConsumptionReference.create("LIQUID_A", UsageBasis.HOURS, "1",
            "🚚".repeat(280), "2026-11-01", catalog).source)
        for (raw in listOf("", "+1", "１", "1\n", "9".repeat(20))) {
            assertThrows(FleetDomainException::class.java) { ConsumptionReference.create("LIQUID_A", UsageBasis.DISTANCE, raw,
                "Synthetic source", "2026-11-01", catalog) }
        }
        assertTrue(AssetRules.activation(profile(), null, emptyList()).toString().startsWith("EnterpriseActivationReadiness"))
    }

    @Test fun `an unstable catalog cannot make contradictory stored product units valid`() {
        val gasCatalog = object : FleetCatalog {
            override fun category(code: String) = catalog.category(code)
            override fun product(code: String) = CatalogProduct(code, "M3")
        }
        val liquidTank = tank()
        val gasTank = Tank.create(TankRole.AUXILIARY, "1", listOf("LIQUID_A"), gasCatalog)
        val gasReference = ConsumptionReference.create("LIQUID_A", UsageBasis.DISTANCE, "8500", "Synthetic maker", "2026-11-01", gasCatalog)
        fun unstable() = object : FleetCatalog {
            private var calls = 0
            override fun category(code: String) = catalog.category(code)
            override fun product(code: String) = CatalogProduct(code, if (++calls == 1) "L" else "M3")
        }
        assertThrows(FleetDomainException::class.java) {
            profile(tanks = listOf(liquidTank), references = listOf(gasReference), currentCatalog = unstable())
        }
        assertThrows(FleetDomainException::class.java) {
            profile(tanks = listOf(liquidTank, gasTank), currentCatalog = unstable())
        }
    }
}
