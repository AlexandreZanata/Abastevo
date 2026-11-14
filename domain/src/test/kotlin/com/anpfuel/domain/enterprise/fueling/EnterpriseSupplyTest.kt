package com.anpfuel.domain.enterprise.fueling

import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetId
import com.anpfuel.domain.enterprise.fleet.AssetRef
import com.anpfuel.domain.enterprise.meter.*
import java.math.BigInteger
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseSupplyTest {
    private fun id(index: Int) = "9d000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private val org = id(1)
    private val other = id(2)
    private val asset = AssetRef(org, AssetId.parse(id(11)))
    private fun input() = CandidateInput(FuelingRef(org, id(131)), asset, EmployeeRef(org, id(21)), MembershipRef(org, id(31)),
        TimeDeclaration(Instant.parse("2026-11-14T12:00:00Z"), "UTC", 0), ProductQuantity.parse("ETHANOL", "L", "150000"),
        RecordedAmounts.parse("97350"), FillLevel.PARTIAL, Payer.COMPANY, PaymentMethod.CASH,
        SupplyReading.Missing(MeterPresence.missing("Display unavailable")))
    private fun fails(expected: FuelFailure = FuelFailure.INVALID, block: () -> Unit) {
        assertEquals(expected, assertThrows(FuelDomainException::class.java, block).failure)
    }
    @Test fun `submit ready candidate requires each explicit fact without inventing a workflow`() {
        val good = input(); val value = FuelingCandidate.create(good)
        assertEquals(97350L, value.amounts.paid); assertEquals(150000L, value.quantity.amount)
        assertNull(value.origin); assertNull(value.establishment); assertNull(value.contractedSupplier)
        for (missing in listOf(good.copy(reference = null), good.copy(asset = null), good.copy(responsible = null), good.copy(author = null),
            good.copy(effective = null), good.copy(quantity = null), good.copy(amounts = null), good.copy(fill = null), good.copy(payer = null),
            good.copy(payment = null), good.copy(reading = null))) fails { FuelingCandidate.create(missing) }
    }
    @Test fun `known units including electricity keep paid facts and opaque original price text`() {
        val price = "  R$ 5,123456\n\u0000" + "x".repeat(1000)
        val amounts = RecordedAmounts.parse("0", "1000000000000", "1000000", price)
        assertEquals(0L, amounts.paid); assertEquals(1_000_000_000_000L, amounts.discount)
        assertEquals(price, amounts.sourcePriceText)
        val electric = FuelingCandidate.create(input().copy(quantity = ProductQuantity.parse("ELECTRICITY", "KWH", "1000000000"), amounts = amounts))
        assertEquals(FuelUnit.KWH, electric.quantity.unit); assertEquals(0L, electric.amounts.paid)
        fails(FuelFailure.PRODUCT_UNIT_MISMATCH) { ProductQuantity.parse("ELECTRICITY", "L", "1") }
        for (raw in listOf("0", "1000000001", "01", "1.5", "１", "-1", "1 ")) fails { ProductQuantity.parse("ETHANOL", "L", raw) }
        for (raw in listOf("-1", "1000000000001", "01", "1.0")) fails { RecordedAmounts.parse(raw) }
        fails { RecordedAmounts.parse("1", unitPrice = "0") }
        fails { RecordedAmounts.parse("1", sourcePriceText = String(charArrayOf('\ud800'))) }
    }
    @Test fun `any supplied unit different from its owned product is a unit mismatch`() {
        for (unit in listOf("", "l", "KG_13", "OTHER", "L ", "M3", "KWH"))
            fails(FuelFailure.PRODUCT_UNIT_MISMATCH) { ProductQuantity.parse("ETHANOL", unit, "1") }
        fails { ProductQuantity.parse("LPG_P13", "KG_13", "1") }
    }
    @Test fun `unknown fill stays partial even at maximum quantity and employee payer stays a declaration`() {
        val value = FuelingCandidate.create(input().copy(fill = FillLevel.UNKNOWN, payer = Payer.EMPLOYEE,
            quantity = ProductQuantity.parse("ARLA_32", "L", "1000000000")))
        assertEquals(FillLevel.UNKNOWN, value.fill); assertEquals(FillLevel.PARTIAL, value.fill.effective)
        assertTrue(value.quantity.product.isAdditive); assertEquals(Payer.EMPLOYEE, value.payer)
        assertEquals(FuelFamily.NONE, value.quantity.product.family)
        for (method in PaymentMethod.entries) assertEquals(method, FuelingCandidate.create(input().copy(payment = method)).payment)
        for (origin in SupplyOrigin.entries) assertEquals(origin, FuelingCandidate.create(input().copy(origin = origin)).origin)
    }
    @Test fun `nominal references reject malformed identities before any source claim`() {
        fails(FuelFailure.SCOPE) { FuelingRef("", id(131)) }
        fails { EmployeeRef(org, "") }
        fails { SupplierRef(org, "00000000-0000-0000-0000-000000000000") }
        fails { PublicStationRef(id(91).uppercase()) }
        fails { DestinationTankRef(asset, "") }
    }
    @Test fun `all private roles and reading asset must match exact candidate ownership`() {
        val good = input()
        for (foreign in listOf(good.copy(asset = AssetRef(other, AssetId.parse(id(11)))), good.copy(responsible = EmployeeRef(other, id(21))),
            good.copy(author = MembershipRef(other, id(31))), good.copy(trip = TripRef(other, id(1))),
            good.copy(contractedSupplier = SupplierRef(other, id(111))), good.copy(establishment = Establishment.Supplier(SupplierRef(other, id(111)))),
            good.copy(establishment = Establishment.InternalTank(InternalSourceTankRef(other, id(81)))))) fails(FuelFailure.SCOPE) { FuelingCandidate.create(foreign) }
        val wrongAsset = AssetRef(org, AssetId.parse(id(12)))
        val reading = ReadingRef(org, SegmentRef(org, MeterRef(org, wrongAsset, id(41), MeterKind.ODOMETER), id(51)), id(61))
        fails(FuelFailure.SCOPE) { FuelingCandidate.create(good.copy(reading = SupplyReading.Reference(reading))) }
        fails(FuelFailure.SCOPE) { FuelingCandidate.create(good.copy(tank = DestinationTankRef(wrongAsset, id(71)))) }
        fails { FuelingCandidate.create(good.copy(tank = DestinationTankRef(asset, id(71)), group = DestinationGroupRef(asset, id(72)))) }
    }
    @Test fun `an observed reference never constructs a received reading or confuses asset and source tanks`() {
        val ref = ReadingRef(org, SegmentRef(org, MeterRef(org, asset, id(41), MeterKind.HOUR_METER), id(51)), id(61))
        val value = FuelingCandidate.create(input().copy(reading = SupplyReading.Reference(ref), tank = DestinationTankRef(asset, id(71)),
            establishment = Establishment.InternalTank(InternalSourceTankRef(org, id(81)))))
        assertSame(ref, (value.reading as SupplyReading.Reference).reference)
        assertEquals(id(71), value.tank!!.id)
        assertEquals(id(81), (value.establishment as Establishment.InternalTank).source.id)
    }
    @Test fun `document issuer remains a separate source revision while optional roles remain independent`() {
        val value = FuelingCandidate.create(input().copy(establishment = Establishment.Station(PublicStationRef(id(91))),
            contractedSupplier = SupplierRef(org, id(111))))
        val document = DocumentIssuerProjection(DocumentRef(org, id(121)), Long.MAX_VALUE, "Synthetic separate issuer" + "x".repeat(1000))
        val roles = FuelingRolesProjection(value, document)
        assertSame(document, roles.issuer)
        assertEquals(id(91), (roles.candidate.establishment as Establishment.Station).station.id)
        assertEquals(id(111), roles.candidate.contractedSupplier!!.id)
        fails(FuelFailure.SCOPE) { FuelingRolesProjection(value, DocumentIssuerProjection(DocumentRef(other, id(121)), 1, "Foreign")) }
        fails { DocumentIssuerProjection(DocumentRef(org, id(121)), 0, "Issuer") }
        fails { DocumentIssuerProjection(DocumentRef(org, id(121)), 1, "\n") }
        fails { Establishment.NotFound(" ") }
        fails { Establishment.NotFound(String(charArrayOf('\ud800'))) }
        assertEquals("Place " + "x".repeat(1000), Establishment.NotFound("Place " + "x".repeat(1000)).name)
    }
    @Test fun `compatible purchased sums are exact keep duplicates and never mix additive with liquid`() {
        val one = ProductQuantity.parse("ETHANOL", "L", "1000000000")
        val sameCategory = ProductQuantity.parse("GASOLINE_REGULAR", "L", "1000000000")
        val additive = ProductQuantity.parse("ARLA_32", "L", "1")
        assertEquals(BigInteger("3000000000"), PurchasedQuantities.compatible(listOf(one, one, sameCategory)).amount)
        assertEquals(2, PurchasedQuantities.byProduct(listOf(one, sameCategory)).size)
        assertTrue(PurchasedQuantities.byProduct(emptyList()).isEmpty())
        fails { PurchasedQuantities.compatible(emptyList()) }
        fails { PurchasedQuantities.compatible(listOf(one, additive)) }
        fails { PurchasedQuantities.compatible(listOf(one, ProductQuantity.parse("CNG", "M3", "1"))) }
        val caller = mutableListOf(one, one)
        val result = PurchasedQuantities.byProduct(caller)
        caller.clear()
        assertEquals(BigInteger("2000000000"), result.single().amount)
    }
    @Test fun `redaction keeps prices labels and private reference identities out of diagnostics`() {
        val value = FuelingCandidate.create(input().copy(establishment = Establishment.NotFound("Synthetic private place")))
        val issuer = DocumentIssuerProjection(DocumentRef(org, id(121)), 1, "Synthetic private issuer")
        val roles = FuelingRolesProjection(value, issuer)
        for (item in listOf(value, input(), value.reference, value.responsible, SupplierRef(org, id(111)), TripRef(org, id(1)),
            DestinationGroupRef(asset, id(72)), InternalSourceTankRef(org, id(81)), PublicStationRef(id(91)), value.reading,
            value.quantity, value.amounts, value.establishment!!, issuer.document, issuer, roles,
            PurchasedQuantities.byProduct(listOf(value.quantity)).single(), PurchasedQuantities.compatible(listOf(value.quantity)))) {
            assertTrue(item.toString().contains("redacted")); assertFalse(item.toString().contains(org))
            assertFalse(item.toString().contains("Synthetic private"))
        }
    }
}
