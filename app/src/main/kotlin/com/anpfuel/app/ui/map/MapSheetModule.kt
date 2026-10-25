package com.anpfuel.app.ui.map

import com.anpfuel.app.mapper.PriceFormatter
import com.anpfuel.application.usecase.community.CommunityPriceGroupsOutcome
import com.anpfuel.application.usecase.community.GetCommunityPriceGroupsUseCase
import com.anpfuel.application.usecase.directory.GetServerStationDetailUseCase
import com.anpfuel.application.usecase.directory.ServerStationDetailOutcome
import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.charging.ChargingCondition
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingObservationGateway
import com.anpfuel.domain.community.CityCommunityFeedReader
import com.anpfuel.domain.community.FeedCity
import com.anpfuel.domain.community.FeedQuery
import com.anpfuel.domain.community.FeedSort
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.model.BackendPriceGroup
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.rule.ActiveSurveyWeekRule
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.maps.engine.MapOverlayMarker
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import java.time.Instant
import java.time.LocalDate
import java.util.Locale
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.CancellationException

/**
 * B-BR-MAP-PRICE-WIRE-01 & B-BR-MAP-PRICE-COND-01: Production sheet price source
 * backed by the real ANP, Community, and EV charging repositories/use cases.
 *
 * - ANP uses station-specific observations in the chosen survey period (local
 *   imported ANP survey first, then backend official price-group section).
 * - Community uses the validated non-expired projection (`BackendCommunitySection`
 *   or `CityCommunityFeedReader`), and EV charging uses `ChargingObservationGateway`.
 * - Conditional prices (`LOYALTY`, `APP`, `CASH`, `OTHER`, `OFF_PEAK`, `PEAK`, `MEMBER`)
 *   preserve their exact `conditionKind` and never silently convert into `STANDARD`.
 * - Incompatible units, divergent products/periods, malformed survey periods,
 *   incompatible EV connectors, and `UNKNOWN` conditions/connectors are rejected.
 * - Exact decimal money is preserved (`BigDecimal` / integer milli-BRL, never float).
 * - Absence (`null` -> `"no-price"`) and lookup failure (throws -> `"load-failed"`)
 *   stay distinct; sources never fall back to each other.
 */
@Singleton
class RealSheetPriceSource @Inject constructor(
    private val userPreferences: UserPreferencesRepository,
    private val vehicles: VehicleRepository,
    private val stationDetail: GetServerStationDetailUseCase,
    private val stationPrices: StationPriceRepository,
    private val priceTables: PriceTableRepository,
    private val catalog: MunicipalityCatalogRepository,
    private val backendPrices: GetCommunityPriceGroupsUseCase,
    private val cityFeed: CityCommunityFeedReader,
    private val chargingObservations: ChargingObservationGateway,
) : SheetPriceSource {

    internal var nowProvider: () -> Instant = { Instant.now() }

    override suspend fun lookup(
        stationUuid: String,
        source: MapPriceSource,
        marker: MapOverlayMarker,
    ): SheetPrice? = lookup(
        SheetPriceRequest(
            stationUuid = stationUuid,
            source = source,
            marker = marker,
        ),
    )

    override suspend fun lookup(request: SheetPriceRequest): SheetPrice? {
        val stationId = request.stationUuid.trim().lowercase(Locale.ROOT)
        require(CANONICAL_UUID_REGEX.matches(stationId)) { "invalid station uuid: ${request.stationUuid}" }

        if (!request.surveyPeriod.isNullOrBlank() && parseSurveyWeek(request.surveyPeriod) == null) {
            return null
        }
        if (request.chargingConnector == ChargingConnector.UNKNOWN) {
            return null
        }

        val prefs = userPreferences.getPreferences()
        val product = request.fuelProduct
            ?: prefs.preferredFuelProduct
            ?: runCatching { vehicles.listAll().firstNotNullOfOrNull { it.fuelProduct } }
                .onFailure { if (it is CancellationException) throw it }
                .getOrNull()
            ?: FuelProduct.GASOLINE_REGULAR
        val wireProduct = WireFuelMapper.toWire(product)
        val defaultUnit = unitForProduct(product)
        val explicitPeriod = if (!request.surveyPeriod.isNullOrBlank()) {
            parseSurveyWeek(request.surveyPeriod) ?: return null
        } else {
            prefs.activeSurveyWeek
        }

        return when (request.source) {
            MapPriceSource.ANP -> lookupAnp(
                stationId = stationId,
                marker = request.marker,
                product = product,
                wireProduct = wireProduct,
                defaultUnit = defaultUnit,
                explicitPeriod = explicitPeriod,
                preferredState = prefs.preferredState,
                preferredMunicipality = prefs.preferredMunicipality,
            )
            MapPriceSource.COMMUNITY -> lookupCommunity(
                stationId = stationId,
                marker = request.marker,
                product = product,
                wireProduct = wireProduct,
                defaultUnit = defaultUnit,
                chargingConnector = request.chargingConnector,
                explicitFuelRequested = request.fuelProduct != null,
                preferredState = prefs.preferredState,
                preferredMunicipality = prefs.preferredMunicipality,
            )
        }
    }

    private suspend fun lookupAnp(
        stationId: String,
        marker: MapOverlayMarker,
        product: FuelProduct,
        wireProduct: String,
        defaultUnit: String,
        explicitPeriod: SurveyWeek?,
        preferredState: BrazilianState?,
        preferredMunicipality: String?,
    ): SheetPrice? {
        if (!marker.fuel) return null

        val importedSurveys = try {
            priceTables.getImportedPriceSurveys()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            emptyList()
        }
        val targetWeek = explicitPeriod ?: ActiveSurveyWeekRule.resolveDisplayWeek(null, importedSurveys)

        var resolvedStation: ServerStation? = null
        var stationStale = false
        var detailError: Throwable? = null
        when (val detailOutcome = stationDetail(stationId)) {
            is ServerStationDetailOutcome.Fresh -> resolvedStation = detailOutcome.station
            is ServerStationDetailOutcome.StaleCache -> {
                resolvedStation = detailOutcome.station
                stationStale = true
            }
            is ServerStationDetailOutcome.Unavailable -> detailError = detailOutcome.cause
            is ServerStationDetailOutcome.Disabled -> Unit
        }

        var localSurveyChecked = false
        val cnpjDigits = resolvedStation?.cnpjNormalized
        if (targetWeek != null && cnpjDigits != null) {
            val state = resolvedStation?.state?.let(::parseState) ?: preferredState
            val municipality = resolveMunicipalityName(
                state = state,
                ibgeCode = resolvedStation?.municipalityCode,
                fallback = preferredMunicipality,
            )
            if (state != null && !municipality.isNullOrBlank()) {
                val localPrice = stationPrices.findStationPrice(
                    state = state,
                    municipality = municipality,
                    fuelProduct = product,
                    surveyWeek = targetWeek,
                    cnpjDigits = cnpjDigits,
                )
                if (localPrice != null && localPrice.fuelProduct == product && localPrice.surveyWeek == targetWeek) {
                    val amount = PriceFormatter.formatAmount(localPrice.price, PT_BR_LOCALE)
                        ?.replace('\u00a0', ' ')
                        ?: "R$ ${localPrice.price.value.toPlainString()}"
                    val periodLabel = "${targetWeek.startDate}..${targetWeek.endDate}"
                    val dateLabel = localPrice.collectedAt?.let { "$periodLabel ($it)" } ?: periodLabel
                    return SheetPrice(
                        product = wireProduct,
                        unit = defaultUnit,
                        amount = amount,
                        source = MapPriceSource.ANP,
                        dateLabel = dateLabel,
                        stale = stationStale,
                        conditionKind = "STANDARD",
                    )
                }
                val hasLocalStationData = try {
                    stationPrices.hasStationData(targetWeek, state, municipality) ||
                        priceTables.findPriceSurveyByWeek(targetWeek)?.hasStationData == true
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {
                    false
                }
                if (hasLocalStationData) {
                    localSurveyChecked = true
                }
            }
        }

        return when (val groupsOutcome = backendPrices(stationId, product)) {
            is CommunityPriceGroupsOutcome.Fresh -> extractOfficialPrice(
                stationId = stationId,
                product = product,
                wireProduct = wireProduct,
                defaultUnit = defaultUnit,
                explicitPeriod = explicitPeriod,
                groups = groupsOutcome.groups.groups,
                stale = false,
            )
            is CommunityPriceGroupsOutcome.StaleCache -> extractOfficialPrice(
                stationId = stationId,
                product = product,
                wireProduct = wireProduct,
                defaultUnit = defaultUnit,
                explicitPeriod = explicitPeriod,
                groups = groupsOutcome.groups.groups,
                stale = true,
            )
            is CommunityPriceGroupsOutcome.Disabled -> {
                if (localSurveyChecked) null
                else if (detailError != null) throw IllegalStateException("anp-station-detail-unavailable", detailError)
                else null
            }
            is CommunityPriceGroupsOutcome.Unavailable -> {
                if (localSurveyChecked) null
                else throw IllegalStateException("anp-price-lookup-failed", groupsOutcome.cause)
            }
        }
    }

    private fun extractOfficialPrice(
        stationId: String,
        product: FuelProduct,
        wireProduct: String,
        defaultUnit: String,
        explicitPeriod: SurveyWeek?,
        groups: List<BackendPriceGroup>,
        stale: Boolean,
    ): SheetPrice? {
        val validCandidates = groups.filter { group ->
            if (!group.stationId.equals(stationId, ignoreCase = true)) return@filter false
            if (group.fuelProductWire != wireProduct) return@filter false
            if (!isUnitCompatibleWithProduct(group.unit, product)) return@filter false
            val cond = normalizeFuelCondition(group.conditionKind) ?: return@filter false
            val official = group.official ?: return@filter false
            if (official.amountMilliBrl !in 1..1_000_000) return@filter false
            val officialWeek = parseSurveyWeek("${official.surveyWeekStart}..${official.surveyWeekEnd}")
                ?: return@filter false
            if (!isValidIsoDay(official.collectedOn)) return@filter false
            if (explicitPeriod != null && officialWeek != explicitPeriod) return@filter false
            cond.isNotEmpty()
        }
        val group = validCandidates
            .sortedBy { fuelConditionPriority(normalizeFuelCondition(it.conditionKind) ?: "OTHER") }
            .firstOrNull()
            ?: return null
        val official = group.official ?: return null
        val conditionKind = normalizeFuelCondition(group.conditionKind) ?: return null
        val amount = formatExactMilliBrl(official.amountMilliBrl)
        val unit = formatWireUnit(group.unit, defaultUnit)
        val dateLabel = "${official.surveyWeekStart}..${official.surveyWeekEnd} (${official.collectedOn})"
        return SheetPrice(
            product = group.fuelProductWire,
            unit = unit,
            amount = amount,
            source = MapPriceSource.ANP,
            dateLabel = dateLabel,
            stale = stale,
            conditionKind = conditionKind,
        )
    }

    private suspend fun lookupCommunity(
        stationId: String,
        marker: MapOverlayMarker,
        product: FuelProduct,
        wireProduct: String,
        defaultUnit: String,
        chargingConnector: ChargingConnector?,
        explicitFuelRequested: Boolean,
        preferredState: BrazilianState?,
        preferredMunicipality: String?,
    ): SheetPrice? {
        val now = nowProvider()
        var anySourceSucceeded = false
        var firstError: Throwable? = null

        val lookupFuel = marker.fuel && (chargingConnector == null || explicitFuelRequested)
        val lookupCharging = marker.charging && (!marker.fuel || chargingConnector != null)

        if (lookupFuel) {
            when (val groupsOutcome = backendPrices(stationId, product)) {
                is CommunityPriceGroupsOutcome.Fresh -> {
                    anySourceSucceeded = true
                    extractCommunityPriceFromGroups(
                        stationId = stationId,
                        product = product,
                        wireProduct = wireProduct,
                        defaultUnit = defaultUnit,
                        groups = groupsOutcome.groups.groups,
                        now = now,
                        stale = false,
                    )?.let { return it }
                }
                is CommunityPriceGroupsOutcome.StaleCache -> {
                    anySourceSucceeded = true
                    extractCommunityPriceFromGroups(
                        stationId = stationId,
                        product = product,
                        wireProduct = wireProduct,
                        defaultUnit = defaultUnit,
                        groups = groupsOutcome.groups.groups,
                        now = now,
                        stale = true,
                    )?.let { return it }
                }
                is CommunityPriceGroupsOutcome.Unavailable -> {
                    firstError = groupsOutcome.cause
                }
                is CommunityPriceGroupsOutcome.Disabled -> Unit
            }

            val feedCity = resolveFeedCity(stationId, preferredState, preferredMunicipality)
            if (feedCity != null) {
                try {
                    val page = cityFeed.read(FeedQuery(feedCity, product, FeedSort.RECENT))
                    anySourceSucceeded = true
                    val item = page.items.firstOrNull {
                        it.stationId.equals(stationId, ignoreCase = true) &&
                            it.fuel == product &&
                            isUnitCompatibleWithProduct(it.unit, product) &&
                            !it.updatedAt.isAfter(now) &&
                            it.expiresAt.isAfter(now)
                    }
                    if (item != null) {
                        return SheetPrice(
                            product = wireProduct,
                            unit = formatWireUnit(item.unit, defaultUnit),
                            amount = formatExactMilliBrl(item.amountMilliBrl),
                            source = MapPriceSource.COMMUNITY,
                            dateLabel = item.updatedAt.toString().take(10),
                            stale = false,
                            conditionKind = "STANDARD",
                        )
                    }
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    if (firstError == null) firstError = e
                }
            }
        }

        if (lookupCharging) {
            try {
                val history = chargingObservations.history(stationId, 20)
                anySourceSucceeded = true
                val validObservations = history.filter { obs ->
                    val milli = obs.priceMilliBrlPerKwh ?: return@filter false
                    if (milli !in 1..10_000_000) return@filter false
                    if (obs.connector == ChargingConnector.UNKNOWN) return@filter false
                    if (chargingConnector != null && obs.connector != chargingConnector) return@filter false
                    if (obs.condition == ChargingCondition.UNKNOWN) return@filter false
                    if (!isValidIsoDay(obs.observedDay)) return@filter false
                    true
                }
                val obs = validObservations
                    .sortedBy { chargingConditionPriority(it.condition) }
                    .firstOrNull()
                val milli = obs?.priceMilliBrlPerKwh
                if (obs != null && milli != null) {
                    return SheetPrice(
                        product = obs.connector.name,
                        unit = "kWh",
                        amount = formatExactMilliBrl(milli),
                        source = MapPriceSource.COMMUNITY,
                        dateLabel = obs.observedDay,
                        stale = false,
                        conditionKind = obs.condition.name,
                    )
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (firstError == null) firstError = e
            }
        }

        if (!anySourceSucceeded && firstError != null) {
            throw IllegalStateException("community-price-lookup-failed", firstError)
        }
        return null
    }

    private fun extractCommunityPriceFromGroups(
        stationId: String,
        product: FuelProduct,
        wireProduct: String,
        defaultUnit: String,
        groups: List<BackendPriceGroup>,
        now: Instant,
        stale: Boolean,
    ): SheetPrice? {
        val validGroups = groups.filter { group ->
            if (!group.stationId.equals(stationId, ignoreCase = true)) return@filter false
            if (group.fuelProductWire != wireProduct) return@filter false
            if (!isUnitCompatibleWithProduct(group.unit, product)) return@filter false
            val cond = normalizeFuelCondition(group.conditionKind) ?: return@filter false
            val comm = group.community ?: return@filter false
            if (comm.availability != "AVAILABLE") return@filter false
            val milli = comm.amountMilliBrl ?: return@filter false
            if (milli !in 1..1_000_000) return@filter false
            val expiresAt = comm.expiresAt
            val lastReceivedAt = comm.lastReceivedAt
            if (expiresAt == null || !expiresAt.isAfter(now)) return@filter false
            if (lastReceivedAt == null || lastReceivedAt.isAfter(now)) return@filter false
            cond.isNotEmpty()
        }
        val group = validGroups
            .sortedBy { fuelConditionPriority(normalizeFuelCondition(it.conditionKind) ?: "OTHER") }
            .firstOrNull()
            ?: return null
        val comm = group.community ?: return null
        val milli = comm.amountMilliBrl ?: return null
        val conditionKind = normalizeFuelCondition(group.conditionKind) ?: return null
        val dateLabel = comm.lastReceivedAt?.toString()?.take(10) ?: "COMMUNITY"
        return SheetPrice(
            product = group.fuelProductWire,
            unit = formatWireUnit(group.unit, defaultUnit),
            amount = formatExactMilliBrl(milli),
            source = MapPriceSource.COMMUNITY,
            dateLabel = dateLabel,
            stale = stale,
            conditionKind = conditionKind,
        )
    }

    private suspend fun resolveFeedCity(
        stationId: String,
        preferredState: BrazilianState?,
        preferredMunicipality: String?,
    ): FeedCity? {
        if (preferredState != null && !preferredMunicipality.isNullOrBlank()) {
            val entry = try {
                catalog.findCatalogEntry(preferredState, preferredMunicipality)
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                null
            }
            val code = entry?.ibgeCode
            if (code != null) {
                return FeedCity(code, preferredState, preferredMunicipality)
            }
        }
        val detail = when (val out = stationDetail(stationId)) {
            is ServerStationDetailOutcome.Fresh -> out.station
            is ServerStationDetailOutcome.StaleCache -> out.station
            else -> null
        } ?: return null
        val state = detail.state?.let(::parseState) ?: return null
        val code = detail.municipalityCode ?: return null
        if (!code.matches(IBGE_REGEX)) return null
        val name = resolveMunicipalityName(state, code, detail.displayName) ?: state.abbreviation
        return FeedCity(code, state, name)
    }

    private suspend fun resolveMunicipalityName(
        state: BrazilianState?,
        ibgeCode: String?,
        fallback: String?,
    ): String? {
        if (state != null && !ibgeCode.isNullOrBlank()) {
            val found = try {
                catalog.getCatalogMunicipalities(state).firstOrNull { it.ibgeCode == ibgeCode }?.municipality
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                null
            }
            if (!found.isNullOrBlank()) return found
        }
        return fallback?.trim()?.takeIf { it.isNotEmpty() }
    }

    companion object {
        private val PT_BR_LOCALE: Locale = Locale.forLanguageTag("pt-BR")
        private val IBGE_REGEX = Regex("^[0-9]{7}$")
        private val ISO_DAY_REGEX = Regex("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")
        private val CANONICAL_UUID_REGEX = Regex(
            "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
        )
        private val ALLOWED_FUEL_CONDITIONS = setOf("STANDARD", "CASH", "LOYALTY", "APP", "OTHER")

        internal fun normalizeFuelCondition(raw: String?): String? {
            val norm = raw?.trim()?.uppercase(Locale.ROOT) ?: return null
            return norm.takeIf { it in ALLOWED_FUEL_CONDITIONS }
        }

        internal fun fuelConditionPriority(conditionKind: String): Int = when (conditionKind) {
            "STANDARD" -> 0
            "CASH" -> 1
            "LOYALTY" -> 2
            "APP" -> 3
            "OTHER" -> 4
            else -> 5
        }

        internal fun chargingConditionPriority(condition: ChargingCondition): Int = when (condition) {
            ChargingCondition.STANDARD -> 0
            ChargingCondition.OFF_PEAK -> 1
            ChargingCondition.PEAK -> 2
            ChargingCondition.MEMBER -> 3
            ChargingCondition.UNKNOWN -> 4
        }

        internal fun isUnitCompatibleWithProduct(wireUnit: String?, product: FuelProduct): Boolean {
            val u = wireUnit?.trim()?.uppercase(Locale.ROOT) ?: return false
            return when (product) {
                FuelProduct.CNG -> u == "M3" || u == "M³"
                FuelProduct.LPG_P13 -> u == "KG_13" || u == "13 KG" || u == "13KG"
                else -> u == "L" || u == "LITRO" || u == "LITRE"
            }
        }

        internal fun isValidIsoDay(raw: String?): Boolean {
            if (raw.isNullOrBlank()) return false
            val trimmed = raw.trim()
            if (!ISO_DAY_REGEX.matches(trimmed)) return false
            return runCatching { LocalDate.parse(trimmed) }.isSuccess
        }

        internal fun formatExactMilliBrl(amountMilliBrl: Long): String {
            require(amountMilliBrl in 1..10_000_000) { "amount_milli_brl out of range" }
            val reais = amountMilliBrl / 1000L
            val frac = (amountMilliBrl % 1000L).toInt()
            return "R$ $reais,%03d".format(Locale.ROOT, frac)
        }

        internal fun unitForProduct(product: FuelProduct): String = when (product) {
            FuelProduct.CNG -> "m³"
            FuelProduct.LPG_P13 -> "13 kg"
            else -> "L"
        }

        internal fun formatWireUnit(wireUnit: String, fallback: String): String = when (wireUnit.trim().uppercase(Locale.ROOT)) {
            "L", "LITRO", "LITRE" -> "L"
            "M3", "M³" -> "m³"
            "KG_13", "13 KG", "13KG" -> "13 kg"
            "KWH" -> "kWh"
            else -> wireUnit.ifBlank { fallback }
        }

        internal fun parseSurveyWeek(raw: String?): SurveyWeek? {
            if (raw.isNullOrBlank()) return null
            val parts = raw.trim().split("..", "/")
            if (parts.size != 2) return null
            return runCatching { SurveyWeek.fromIsoDates(parts[0].trim(), parts[1].trim()) }.getOrNull()
        }

        internal fun parseState(raw: String): BrazilianState? =
            BrazilianState.entries.firstOrNull { it.abbreviation.equals(raw.trim(), ignoreCase = true) }
    }
}

@Module
@InstallIn(SingletonComponent::class)
object MapSheetModule {
    @Provides
    @Singleton
    fun provideSheetPriceSource(impl: RealSheetPriceSource): SheetPriceSource = impl
}
