package com.anpfuel.app.ui.stations

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.app.mapper.StationPriceUiMapper
import com.anpfuel.app.mapper.SurveyWeekFormatter
import com.anpfuel.app.ui.model.StationDetailUiModel
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.application.usecase.community.CommunityPriceGroupsOutcome
import com.anpfuel.application.usecase.community.GetCityCommunityFeedUseCase
import com.anpfuel.application.usecase.community.GetCommunityPriceGroupsUseCase
import com.anpfuel.application.usecase.directory.GetServerStationDetailUseCase
import com.anpfuel.domain.community.CommunityFeedItem
import com.anpfuel.domain.community.FeedQuery
import com.anpfuel.domain.community.FeedSort
import com.anpfuel.application.usecase.directory.ResolveStationByCnpjUseCase
import com.anpfuel.application.usecase.directory.ServerStationDetailOutcome
import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.price.StationPricesOutcome
import com.anpfuel.application.usecase.profile.GetStationProfileUseCase
import com.anpfuel.application.usecase.profile.StationProfileOutcome
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationDetailRule
import com.anpfuel.domain.discovery.StationDetailState
import com.anpfuel.domain.discovery.StationPageIdentity
import com.anpfuel.domain.discovery.StationRowFreshness
import com.anpfuel.domain.model.BackendPriceGroup
import com.anpfuel.domain.profile.StationProfile
import com.anpfuel.domain.valueobject.FuelProduct
import dagger.hilt.android.lifecycle.HiltViewModel
import java.time.LocalDate
import java.util.Locale
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

data class StationPageState(
    val fuel: FuelProduct = FuelProduct.GASOLINE_REGULAR,
    val name: String = "",
    val address: String = "",
    val navigationQuery: String? = null,
    val brand: String? = null,
    val location: String = "",
    val localDetail: StationDetailUiModel? = null,
    val canonical: ServerStation? = null,
    val profile: StationProfile? = null,
    val officialGroups: List<BackendPriceGroup> = emptyList(),
    val communityItem: CommunityFeedItem? = null,
    val loading: Boolean = true,
    val unavailable: Boolean = false,
    val identityStale: Boolean = false,
    val pricesStale: Boolean = false,
    val priceUnavailable: Boolean = false,
    val accountId: String = "",
) {
    val canParticipate: Boolean get() = canonical != null && !identityStale && !loading
}

/** Restorable identity/fuel route; sealed credentials and price snapshots never enter navigation. */
@HiltViewModel
class StationPageViewModel @Inject constructor(
    private val saved: SavedStateHandle,
    private val localPrices: GetStationPricesUseCase,
    private val resolve: ResolveStationByCnpjUseCase,
    private val directory: GetServerStationDetailUseCase,
    private val prices: GetCommunityPriceGroupsUseCase,
    private val profiles: GetStationProfileUseCase,
    private val cityFeed: GetCityCommunityFeedUseCase,
    private val auth: AuthFlow,
) : ViewModel() {
    private val identity = StationPageIdentity.parse(saved.get<String>("stationKey") ?: saved.get<String>("stationId").orEmpty())
    private val mutable = MutableStateFlow(StationPageState(fuel = saved.get<String>("fuelProduct")
        ?.let { runCatching { FuelProduct.valueOf(it) }.getOrNull() } ?: FuelProduct.GASOLINE_REGULAR))
    val state = mutable.asStateFlow()
    internal var ioDispatcher: CoroutineDispatcher = Dispatchers.IO
    private var generation = 0
    private var accountGeneration = 0
    private var loadJob: Job? = null

    fun selectFuel(fuel: FuelProduct, locale: Locale) {
        if (fuel == mutable.value.fuel) return
        saved["fuelProduct"] = fuel.name
        mutable.update { it.copy(fuel = fuel, localDetail = null, officialGroups = emptyList(), communityItem = null) }
        load(locale)
    }

    fun refreshAccount() {
        val request = ++accountGeneration
        viewModelScope.launch {
            val account = withContext(ioDispatcher) {
                try { (auth.refreshSession() as? AuthApiResult.Ok)?.value?.accountId.orEmpty() }
                catch (cancelled: CancellationException) { throw cancelled }
                catch (_: Exception) { "" }
            }
            if (request == accountGeneration) mutable.update { it.copy(accountId = account) }
        }
    }

    fun load(locale: Locale) {
        val request = ++generation
        loadJob?.cancel()
        val fuel = mutable.value.fuel
        mutable.update { it.copy(loading = true, unavailable = false, localDetail = null,
            officialGroups = emptyList(), communityItem = null, profile = null, priceUnavailable = false, pricesStale = false) }
        if (identity == null) {
            mutable.update { it.copy(loading = false, unavailable = true, canonical = null) }
            return
        }
        refreshAccount()
        loadJob = viewModelScope.launch {
            val legacy = withContext(ioDispatcher) {
                try { localPrices(fuelProduct = fuel) as? StationPricesOutcome.Success }
                catch (cancelled: CancellationException) { throw cancelled }
                catch (_: Exception) { null }
            }
            val cnpj = (identity as? StationPageIdentity.Legacy)?.cnpj
            val local = legacy?.stations?.firstOrNull { it.station.cnpj.digits == cnpj }
            if (request != generation) return@launch
            if (local != null) {
                val resolved = StationDetailRule.resolve(listOf(local), legacy.surveyWeek, LocalDate.now(), false)
                val reference = resolved as? StationDetailState.AnpReference
                val detail = reference?.let {
                    StationDetailUiModel(StationPriceUiMapper.toUiModel(local, locale, legacy.state, legacy.municipality),
                        SurveyWeekFormatter.formatRange(legacy.surveyWeek, locale),
                        it.rows.first().freshness == StationRowFreshness.STALE,
                        it.rows.first().freshness == StationRowFreshness.UNKNOWN, it.communityDisputed)
                }
                mutable.update { it.copy(name = local.station.displayName(), address = local.station.address,
                    brand = local.station.brand, navigationQuery = detail?.station?.navigationQuery, location = "${legacy.municipality}, ${legacy.state.abbreviation}", localDetail = detail) }
            }
            val outcome = withContext(ioDispatcher) {
                try {
                    when (identity) {
                        is StationPageIdentity.Canonical -> directory(identity.stationId)
                        is StationPageIdentity.Legacy -> resolve(identity.cnpj)
                    }
                } catch (cancelled: CancellationException) { throw cancelled }
                catch (error: Exception) { ServerStationDetailOutcome.Unavailable(error) }
            }
            if (request != generation) return@launch
            val station = when (outcome) {
                is ServerStationDetailOutcome.Fresh -> outcome.station
                is ServerStationDetailOutcome.StaleCache -> outcome.station
                else -> null
            }?.takeIf { found ->
                when (identity) {
                    is StationPageIdentity.Canonical -> found.stationId == identity.stationId
                    is StationPageIdentity.Legacy -> found.cnpjNormalized == identity.cnpj
                }
            }
            mutable.update { it.copy(canonical = station, identityStale = outcome is ServerStationDetailOutcome.StaleCache,
                name = it.name.ifEmpty { station?.displayName.orEmpty() },
                location = it.location.ifEmpty { station?.state.orEmpty() },
                unavailable = station == null && it.name.isEmpty(), priceUnavailable = station == null) }
            if (station != null) {
                val groups = withContext(ioDispatcher) {
                    try { prices(station.stationId, fuel) }
                    catch (cancelled: CancellationException) { throw cancelled }
                    catch (error: Exception) { CommunityPriceGroupsOutcome.Unavailable(error) }
                }
                val profile = withContext(ioDispatcher) {
                    try { (profiles(station.stationId) as? StationProfileOutcome.Found)?.profile }
                    catch (cancelled: CancellationException) { throw cancelled }
                    catch (_: Exception) { null }
                }
                if (request != generation) return@launch
                val found = when (groups) {
                    is CommunityPriceGroupsOutcome.Fresh -> groups.groups
                    is CommunityPriceGroupsOutcome.StaleCache -> groups.groups
                    else -> null
                }
                mutable.update { it.copy(profile = profile?.takeIf { p -> p.stationId == station.stationId },
                    officialGroups = found?.groups.orEmpty().filter { g -> g.stationId == station.stationId && g.fuelProductWire == com.anpfuel.data.mapper.WireFuelMapper.toWire(fuel) },
                    pricesStale = groups is CommunityPriceGroupsOutcome.StaleCache,
                    priceUnavailable = groups is CommunityPriceGroupsOutcome.Unavailable || groups is CommunityPriceGroupsOutcome.Disabled) }
                // Public city feed carries the actual community price (amount +
                // supporters/confirmations) for this station+fuel when available.
                // The backend price-groups community section stays null (P04),
                // so this lookup is the only honest community source here.
                val community = withContext(ioDispatcher) {
                    try {
                        val city = cityFeed.city() ?: return@withContext null
                        val page = cityFeed(FeedQuery(city, fuel, FeedSort.RECENT))
                        page.items.firstOrNull { it.stationId == station.stationId && it.fuel == fuel }
                    } catch (cancelled: CancellationException) { throw cancelled }
                    catch (_: Exception) { null }
                }
                if (request != generation) return@launch
                if (community != null) mutable.update { it.copy(communityItem = community) }
            }
            if (request == generation) mutable.update { it.copy(loading = false) }
        }
    }
}
