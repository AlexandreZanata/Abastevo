package com.anpfuel.domain.enterprise.fueling

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.EnterpriseValueException
import com.anpfuel.domain.enterprise.EnterpriseValues
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fleet.AssetRef
import com.anpfuel.domain.enterprise.meter.MembershipRef
import com.anpfuel.domain.enterprise.meter.MeterPresence
import com.anpfuel.domain.enterprise.meter.ReadingRef
import java.math.BigInteger

private fun identifier(raw: String) { try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw invalid() } }
private fun organization(raw: String) { try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw scope() } }
private fun text(raw: String, label: Boolean) {
    if (label && raw.isBlank()) throw invalid()
    val points = raw.codePoints().iterator()
    while (points.hasNext()) {
        val point = points.nextInt()
        if (point in 0xd800..0xdfff || label && Character.isISOControl(point)) throw invalid()
    }
}

/** Supplied scoped identities never authenticate existence, membership or permission. */
sealed class ScopedFuelRef(val organizationId: String, val id: String) {
    init { organization(organizationId); identifier(id) }
    final override fun equals(other: Any?): Boolean = other is ScopedFuelRef && javaClass == other.javaClass && organizationId == other.organizationId && id == other.id
    final override fun hashCode(): Int = 31 * (31 * javaClass.hashCode() + organizationId.hashCode()) + id.hashCode()
    final override fun toString(): String = "EnterpriseScopedFuelRef(redacted)"
}
class FuelingRef(organizationId: String, id: String) : ScopedFuelRef(organizationId, id)
class EmployeeRef(organizationId: String, id: String) : ScopedFuelRef(organizationId, id)
class SupplierRef(organizationId: String, id: String) : ScopedFuelRef(organizationId, id)
class TripRef(organizationId: String, id: String) : ScopedFuelRef(organizationId, id)
class DocumentRef(organizationId: String, id: String) : ScopedFuelRef(organizationId, id)
class InternalSourceTankRef(organizationId: String, id: String) : ScopedFuelRef(organizationId, id)
class DestinationTankRef(val asset: AssetRef, val id: String) {
    init { identifier(id) }
    override fun toString(): String = "EnterpriseDestinationTankRef(redacted)"
}
class DestinationGroupRef(val asset: AssetRef, val id: String) {
    init { identifier(id) }
    override fun toString(): String = "EnterpriseDestinationGroupRef(redacted)"
}
class PublicStationRef(val id: String) {
    init { identifier(id) }
    override fun toString(): String = "EnterprisePublicStationRef(redacted)"
}

enum class FillLevel { FULL, PARTIAL, UNKNOWN;
    val effective: FillLevel get() = if (this == UNKNOWN) PARTIAL else this
    companion object { fun parse(raw: String): FillLevel = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}
enum class Payer { COMPANY, EMPLOYEE;
    companion object { fun parse(raw: String): Payer = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}
enum class PaymentMethod { CASH, PIX, DEBIT_CARD, CREDIT_CARD, FUEL_CARD, SUPPLIER_ACCOUNT, AGREEMENT_VOUCHER, INTERNAL_STOCK, OTHER;
    companion object { fun parse(raw: String): PaymentMethod = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}
enum class SupplyOrigin { APP, PANEL, IMPORT, AGREEMENT, INTERNAL_TANK;
    companion object { fun parse(raw: String): SupplyOrigin = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}
enum class PlaceKind { CATALOG_STATION, ORG_SUPPLIER, INTERNAL_TANK, NOT_FOUND;
    companion object { fun parse(raw: String): PlaceKind = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}
sealed class Establishment(val kind: PlaceKind) {
    class Station(val station: PublicStationRef) : Establishment(PlaceKind.CATALOG_STATION)
    class Supplier(val supplier: SupplierRef) : Establishment(PlaceKind.ORG_SUPPLIER)
    class InternalTank(val source: InternalSourceTankRef) : Establishment(PlaceKind.INTERNAL_TANK)
    class NotFound(val name: String) : Establishment(PlaceKind.NOT_FOUND) { init { text(name, label = true) } }
    final override fun toString(): String = "EnterpriseEstablishment(redacted)"
}
sealed class SupplyReading {
    class Reference(val reference: ReadingRef) : SupplyReading()
    class Missing(val missing: MeterPresence.Missing) : SupplyReading()
    final override fun toString(): String = "EnterpriseSupplyReading(redacted)"
}
class ProductQuantity private constructor(val product: FuelProduct, val amount: Long) {
    val unit: FuelUnit get() = product.unit
    override fun toString(): String = "EnterpriseProductQuantity(redacted)"
    companion object {
        fun parse(product: String, unit: String, amount: String): ProductQuantity {
            val owned = FuelProducts.get(product)
            if (unit != owned.unit.name) throw FuelDomainException(FuelFailure.PRODUCT_UNIT_MISMATCH)
            val value = try { EnterpriseValues.parseQuantityMilli(amount) } catch (_: EnterpriseValueException) { throw invalid() }
            return ProductQuantity(owned, value)
        }
    }
}
class RecordedAmounts private constructor(val paid: Long, val discount: Long?, val unitPrice: Long?, val sourcePriceText: String?) {
    override fun toString(): String = "EnterpriseRecordedAmounts(redacted)"
    companion object {
        fun parse(paid: String, discount: String? = null, unitPrice: String? = null, sourcePriceText: String? = null): RecordedAmounts {
            sourcePriceText?.let { text(it, label = false) }
            return try { RecordedAmounts(EnterpriseValues.parseCentavos(paid), discount?.let(EnterpriseValues::parseCentavos),
                unitPrice?.let(EnterpriseValues::parseUnitPriceMilliBRL), sourcePriceText) }
            catch (_: EnterpriseValueException) { throw invalid() }
        }
    }
}
data class CandidateInput(val reference: FuelingRef? = null, val asset: AssetRef? = null, val responsible: EmployeeRef? = null,
    val author: MembershipRef? = null, val effective: TimeDeclaration? = null, val quantity: ProductQuantity? = null,
    val amounts: RecordedAmounts? = null, val fill: FillLevel? = null, val payer: Payer? = null, val payment: PaymentMethod? = null,
    val reading: SupplyReading? = null, val origin: SupplyOrigin? = null, val tank: DestinationTankRef? = null,
    val group: DestinationGroupRef? = null, val trip: TripRef? = null, val establishment: Establishment? = null,
    val contractedSupplier: SupplierRef? = null) {
    override fun toString(): String = "EnterpriseCandidateInput(redacted)"
}
class FuelingCandidate private constructor(val reference: FuelingRef, val asset: AssetRef, val responsible: EmployeeRef,
    val author: MembershipRef, val effective: TimeDeclaration, val quantity: ProductQuantity, val amounts: RecordedAmounts,
    val fill: FillLevel, val payer: Payer, val payment: PaymentMethod, val reading: SupplyReading, val origin: SupplyOrigin?,
    val tank: DestinationTankRef?, val group: DestinationGroupRef?, val trip: TripRef?, val establishment: Establishment?, val contractedSupplier: SupplierRef?) {
    override fun toString(): String = "EnterpriseFuelingCandidate(redacted)"
    companion object {
        fun create(input: CandidateInput): FuelingCandidate {
            val reference = input.reference ?: throw invalid()
            val asset = input.asset ?: throw invalid()
            val responsible = input.responsible ?: throw invalid()
            val author = input.author ?: throw invalid()
            val declared = input.effective ?: throw invalid()
            val quantity = input.quantity ?: throw invalid()
            val amounts = input.amounts ?: throw invalid()
            val fill = input.fill ?: throw invalid()
            val payer = input.payer ?: throw invalid()
            val payment = input.payment ?: throw invalid()
            val reading = input.reading ?: throw invalid()
            val org = reference.organizationId
            if (asset.organizationId != org || responsible.organizationId != org || author.organizationId != org ||
                input.trip != null && input.trip.organizationId != org || input.contractedSupplier != null && input.contractedSupplier.organizationId != org) throw scope()
            if (input.tank != null && input.group != null) throw invalid()
            if (input.tank != null && input.tank.asset != asset || input.group != null && input.group.asset != asset) throw scope()
            if (reading is SupplyReading.Reference && (reading.reference.organizationId != org || reading.reference.segment.meter.asset != asset)) throw scope()
            when (val place = input.establishment) {
                is Establishment.Supplier -> if (place.supplier.organizationId != org) throw scope()
                is Establishment.InternalTank -> if (place.source.organizationId != org) throw scope()
                is Establishment.Station, is Establishment.NotFound, null -> Unit
            }
            val effective = try { EnterpriseTime.validateDeclaration(declared) } catch (_: EnterpriseTimeException) { throw invalid() }
            return FuelingCandidate(reference, asset, responsible, author, effective, quantity, amounts, fill, payer, payment,
                reading, input.origin, input.tank, input.group, input.trip, input.establishment, input.contractedSupplier)
        }
    }
}
/** A transient DOC read, kept separate from immutable fueling truth. */
class DocumentIssuerProjection(val document: DocumentRef, val sourceRevision: Long, val issuerLabel: String) {
    init { if (sourceRevision <= 0) throw invalid(); text(issuerLabel, label = true) }
    override fun toString(): String = "EnterpriseDocumentIssuerProjection(redacted)"
}
class FuelingRolesProjection(val candidate: FuelingCandidate, val issuer: DocumentIssuerProjection?) {
    init { if (issuer != null && issuer.document.organizationId != candidate.reference.organizationId) throw scope() }
    override fun toString(): String = "EnterpriseFuelingRolesProjection(redacted)"
}
class ProductTotal internal constructor(val product: FuelProduct, val amount: BigInteger) {
    override fun toString(): String = "EnterpriseProductTotal(redacted)"
}
class CompatibleTotal internal constructor(val category: FuelCategory, val unit: FuelUnit, val amount: BigInteger) {
    override fun toString(): String = "EnterpriseCompatibleTotal(redacted)"
}
object PurchasedQuantities {
    fun byProduct(items: List<ProductQuantity>): List<ProductTotal> {
        val values = items.toList()
        val totals = mutableMapOf<FuelProduct, BigInteger>()
        for (item in values) totals[item.product] = (totals[item.product] ?: BigInteger.ZERO) + BigInteger.valueOf(item.amount)
        return totals.entries.sortedBy { it.key.name }.map { ProductTotal(it.key, it.value) }
    }
    fun compatible(items: List<ProductQuantity>): CompatibleTotal {
        val values = items.toList()
        val first = values.firstOrNull() ?: throw invalid()
        val category = first.product.category
        val unit = first.unit
        if (values.any { it.product.category != category || it.unit != unit }) throw invalid()
        val amount = values.fold(BigInteger.ZERO) { sum, item -> sum + BigInteger.valueOf(item.amount) }
        return CompatibleTotal(category, unit, amount)
    }
}
