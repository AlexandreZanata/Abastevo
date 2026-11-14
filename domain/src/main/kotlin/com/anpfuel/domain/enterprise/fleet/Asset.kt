package com.anpfuel.domain.enterprise.fleet

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseValueException
import com.anpfuel.domain.enterprise.EnterpriseValues
import java.time.LocalDate
import java.time.format.DateTimeParseException

enum class FleetFailure { INVALID, SCOPE, KIND_LOCKED }

class FleetDomainException(val failure: FleetFailure = FleetFailure.INVALID) :
    IllegalArgumentException("fleet: ${failure.name.lowercase()}")

private fun invalid(): FleetDomainException = FleetDomainException()
private fun scopeDenied(): FleetDomainException = FleetDomainException(FleetFailure.SCOPE)

/** Deliberately distinct from personal DomainId; parsing does not prove table provenance. */
class AssetId private constructor(val value: String) {
    override fun equals(other: Any?): Boolean = other is AssetId && value == other.value
    override fun hashCode(): Int = value.hashCode()
    override fun toString(): String = "EnterpriseAssetId(redacted)"

    companion object {
        fun parse(raw: String): AssetId = AssetId(reference(raw))
    }
}

/** The private organization is explicit; personal vehicles have no conversion here. */
class AssetRef(organizationId: String, val assetId: AssetId) {
    val organizationId: String = try { EnterprisePartition.requireId(organizationId) } catch (_: IllegalArgumentException) { throw scopeDenied() }
    fun requireOrganization(requested: String) {
        if (requested != organizationId) throw scopeDenied()
    }
    override fun equals(other: Any?): Boolean = other is AssetRef && organizationId == other.organizationId && assetId == other.assetId
    override fun hashCode(): Int = 31 * organizationId.hashCode() + assetId.hashCode()
    override fun toString(): String = "EnterpriseAssetRef(redacted)"
}

private fun reference(raw: String): String = try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw invalid() }

object FleetIdentifiers {
    fun plate(raw: String): String {
        if (raw.isEmpty() || raw.any { it !in 'a'..'z' && it !in 'A'..'Z' && it !in '0'..'9' && it != ' ' && it != '-' }) throw invalid()
        return comparison(raw).also { if (it.length != 7) throw invalid() }
    }

    fun fleetNumber(raw: String): String {
        if (raw.isEmpty() || raw.length > 20 || raw.any { it !in ' '..'~' }) throw invalid()
        return comparison(raw).also { if (it.isEmpty()) throw invalid() }
    }

    private fun comparison(raw: String): String = buildString {
        for (character in raw) when {
            character == ' ' || character == '-' -> Unit
            character in 'a'..'z' -> append(character - ('a' - 'A'))
            else -> append(character)
        }
    }
}

enum class AssetKind {
    VEHICLE, MACHINE, IMPLEMENT;
    companion object { fun parse(raw: String): AssetKind = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}
enum class UsageBasis {
    DISTANCE, HOURS;
    companion object { fun parse(raw: String): UsageBasis = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}
enum class AssetLifecycle {
    DRAFT, ACTIVE, INACTIVE, DISPOSED, ARCHIVED;
    companion object { fun parse(raw: String): AssetLifecycle = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}
enum class TankRole {
    MAIN, AUXILIARY, ADDITIVE, BATTERY;
    companion object { fun parse(raw: String): TankRole = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}

data class CatalogCategory(val code: String) {
    override fun toString(): String = "EnterpriseCatalogCategory(redacted)"
}
data class CatalogProduct(val code: String, val unit: String) {
    override fun toString(): String = "EnterpriseCatalogProduct(redacted)"
}

/** The owning product/category catalog supplies descriptors; this is not a copied personal enum. */
interface FleetCatalog {
    fun category(code: String): CatalogCategory?
    fun product(code: String): CatalogProduct?
}

private fun product(code: String, catalog: FleetCatalog): CatalogProduct {
    metadata(code, Int.MAX_VALUE, required = true)
    val value = catalog.product(code) ?: throw invalid()
    if (value.code != code || value.unit !in setOf("L", "M3", "KWH")) throw invalid()
    return value
}

class Tank private constructor(val role: TankRole, val capacityMilli: Long, val unit: String, products: List<String>) {
    private val accepted = products.toList()
    val products: List<String> get() = accepted.toList()
    fun accepts(code: String): Boolean = code in accepted
    override fun toString(): String = "EnterpriseTank(redacted)"

    companion object {
        fun create(role: TankRole, capacity: String, products: List<String>, catalog: FleetCatalog): Tank {
            val quantity = try { EnterpriseValues.parseQuantityMilli(capacity) } catch (_: EnterpriseValueException) { throw invalid() }
            if (products.isEmpty()) throw invalid()
            products.forEach { metadata(it, Int.MAX_VALUE, required = true) }
            // Go sorts valid UTF-8 by scalar value; JVM's default UTF-16 ordering differs.
            val codes = products.distinct().sortedWith { left, right ->
                var a = 0
                var b = 0
                var comparison = 0
                while (a < left.length && b < right.length) {
                    val first = left.codePointAt(a)
                    val second = right.codePointAt(b)
                    comparison = first.compareTo(second)
                    if (comparison != 0) break
                    a += Character.charCount(first)
                    b += Character.charCount(second)
                }
                if (comparison != 0) comparison else (left.length - a).compareTo(right.length - b)
            }
            val descriptors = codes.map { product(it, catalog) }
            val unit = descriptors.first().unit
            if (descriptors.any { it.unit != unit }) throw invalid()
            return Tank(role, quantity, unit, codes)
        }
    }
}

/** A positive integer reference per product, never a measured consumption or a float. */
class ConsumptionReference private constructor(
    val productCode: String,
    val basis: UsageBasis,
    val value: Long,
    val unit: String,
    val source: String,
    val date: String,
) {
    override fun toString(): String = "EnterpriseConsumptionReference(redacted)"

    companion object {
        fun create(productCode: String, basis: UsageBasis, rawValue: String, source: String, date: String, catalog: FleetCatalog): ConsumptionReference {
            if (rawValue.isEmpty() || rawValue.length > 19 || rawValue[0] !in '1'..'9' || rawValue.any { it !in '0'..'9' }) throw invalid()
            val value = rawValue.toLongOrNull()?.takeIf { it > 0 } ?: throw invalid()
            metadata(source, 280, required = true)
            if (!Regex("[0-9]{4}-[0-9]{2}-[0-9]{2}").matches(date)) throw invalid()
            val parsed = try { LocalDate.parse(date) } catch (_: DateTimeParseException) { throw invalid() }
            if (parsed.year !in 1..9999) throw invalid()
            return ConsumptionReference(productCode, basis, value, product(productCode, catalog).unit, source, date)
        }
    }
}

private fun metadata(raw: String, maximum: Int, required: Boolean): String {
    var offset = 0
    var count = 0
    while (offset < raw.length) {
        val character = raw[offset]
        if (character.isHighSurrogate()) {
            if (offset + 1 >= raw.length || !raw[offset + 1].isLowSurrogate()) throw invalid()
            offset += 2
        } else {
            if (character.isLowSurrogate() || character.isISOControl()) throw invalid()
            offset++
        }
        count++
        if (count > maximum) throw invalid()
    }
    if (required && raw.isBlank()) throw invalid()
    return raw
}

class AssetProfile(
    val reference: AssetRef,
    val kind: AssetKind,
    val categoryCode: String,
    val name: String,
    val fleetNumber: String = "",
    val plate: String = "",
    serial: String = "",
    val basis: UsageBasis,
    tanks: List<Tank> = emptyList(),
    references: List<ConsumptionReference> = emptyList(),
    catalog: FleetCatalog,
) {
    val fleetKey: String = if (fleetNumber.isEmpty()) "" else FleetIdentifiers.fleetNumber(fleetNumber)
    val plateKey: String = if (plate.isEmpty()) "" else FleetIdentifiers.plate(plate)
    val serial: String = metadata(serial, 40, required = false)
    private val storedTanks = tanks.toList()
    private val storedReferences = references.toList()
    val tanks: List<Tank> get() = storedTanks.toList()
    val consumptionReferences: List<ConsumptionReference> get() = storedReferences.toList()

    init {
        metadata(categoryCode, Int.MAX_VALUE, required = true)
        if (catalog.category(categoryCode)?.code != categoryCode) throw invalid()
        metadata(name, 80, required = true)
        if (fleetKey.isEmpty() && plateKey.isEmpty() && this.serial.isBlank()) throw invalid()
        if (storedReferences.map { it.productCode }.distinct().size != storedReferences.size) throw invalid()
        if (storedTanks.any { tank -> tank.products.any { product(it, catalog).unit != tank.unit } }) throw invalid()
        if (storedReferences.any { product(it.productCode, catalog).unit != it.unit }) throw invalid()
        val accepted = mutableMapOf<String, String>()
        for (tank in storedTanks) for (code in tank.products) {
            val previous = accepted.putIfAbsent(code, tank.unit)
            if (previous != null && previous != tank.unit) throw invalid()
        }
        if (storedReferences.any { accepted[it.productCode] != it.unit }) throw invalid()
    }

    override fun toString(): String = "EnterpriseAssetProfile(redacted)"
}

class UnitFact(organizationId: String, unitId: String) {
    val organizationId = try { EnterprisePartition.requireId(organizationId) } catch (_: IllegalArgumentException) { throw scopeDenied() }
    val unitId = reference(unitId)
    override fun toString(): String = "EnterpriseAssetUnitFact(redacted)"
}

class MeterFact(val asset: AssetRef, meterId: String, val inService: Boolean) {
    val meterId = reference(meterId)
    override fun toString(): String = "EnterpriseAssetMeterFact(redacted)"
}

enum class ActivationMissing { UNIT, TANK, METER }

class ActivationReadiness internal constructor(missing: List<ActivationMissing>) {
    private val needed = missing.toList()
    val missing: List<ActivationMissing> get() = needed.toList()
    val ready: Boolean get() = needed.isEmpty()
    override fun toString(): String = "EnterpriseActivationReadiness(ready=$ready)"
}

object AssetRules {
    fun activation(profile: AssetProfile, unit: UnitFact?, meters: List<MeterFact>): ActivationReadiness {
        if (unit != null && unit.organizationId != profile.reference.organizationId || meters.any { it.asset != profile.reference }) throw scopeDenied()
        val missing = buildList {
            if (unit == null) add(ActivationMissing.UNIT)
            if (profile.kind != AssetKind.IMPLEMENT) {
                if (profile.tanks.isEmpty()) add(ActivationMissing.TANK)
                if (meters.none { it.inService }) add(ActivationMissing.METER)
            }
        }
        return ActivationReadiness(missing)
    }

    fun kindAfterChange(current: AssetKind, proposed: AssetKind, hasAcceptedOperation: Boolean): AssetKind {
        if (current != proposed && hasAcceptedOperation) throw FleetDomainException(FleetFailure.KIND_LOCKED)
        return proposed
    }
}
