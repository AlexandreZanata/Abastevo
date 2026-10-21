package com.anpfuel.app.ui.home

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.price.GetMunicipalityPricesUseCase
import com.anpfuel.application.usecase.readiness.GetDataReadinessUseCase
import com.anpfuel.application.usecase.sync.SyncPriceTablesUseCase
import com.anpfuel.application.usecase.vehicle.GetTankFillCostEstimatesUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.data.local.preferences.HomeBenefitsVisibilityStore
import com.anpfuel.data.local.preferences.HomeVehiclePresenceStore
import com.anpfuel.app.mapper.TankFillCostUiMapper
import com.anpfuel.app.ui.model.AveragePriceUiModel
import com.anpfuel.app.ui.model.TankFillCostEstimateUiModel
import com.anpfuel.domain.event.SyncRequestSource
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.state.DataReadinessState
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.SurveyWeek
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.Locale
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

enum class HomePriceSource { ANP, COMMUNITY }

data class HomeUiState(
    val priceSource: HomePriceSource = HomePriceSource.ANP,
    val communityPrices: List<AveragePriceUiModel> = emptyList(),
    val communityLoading: Boolean = false,
    val communityFailed: Boolean = false,
    val communityStale: Boolean = false,
    val communityGeneratedAt: java.time.Instant? = null,
    val communityUpdatedAt: java.time.Instant? = null,
    val communityExpiresAt: java.time.Instant? = null,
    val communityCity: com.anpfuel.domain.community.FeedCity? = null,
    val isLoading: Boolean = true,
    val isRefreshing: Boolean = false,
    val readiness: DataReadinessState = DataReadinessState.EMPTY,
    val isOffline: Boolean = false,
    val municipality: String? = null,
    val state: BrazilianState? = null,
    val surveyWeek: SurveyWeek? = null,
    val prices: List<AveragePriceUiModel> = emptyList(),
    val tankFillCostEstimates: List<TankFillCostEstimateUiModel> = emptyList(),
    val priceSort: HomePriceSortOrder = HomePriceSortOrder.DEFAULT,
    val hasVehicle: Boolean = false,
    val showBenefits: Boolean = true,
    val hasLocation: Boolean = false,
    val hasCachedData: Boolean = false,
    val isEmptyMunicipality: Boolean = false,
    val dataAvailability: DataAvailability? = null,
    val operationalNote: String? = null,
    val error: AppError? = null,
    val errorMessage: String? = null,
)

@HiltViewModel
class HomeViewModel @Inject constructor(
    private val getDataReadinessUseCase: GetDataReadinessUseCase,
    private val getMunicipalityPricesUseCase: GetMunicipalityPricesUseCase,
    private val selectLocationUseCase: SelectLocationUseCase,
    private val syncPriceTablesUseCase: SyncPriceTablesUseCase,
    private val listVehiclesUseCase: ListVehiclesUseCase,
    private val getTankFillCostEstimatesUseCase: GetTankFillCostEstimatesUseCase,
    private val homeVehiclePresenceStore: HomeVehiclePresenceStore,
    private val homeBenefitsVisibilityStore: HomeBenefitsVisibilityStore,
    observeNetworkConnectivityUseCase: ObserveNetworkConnectivityUseCase,
    private val communitySummary: com.anpfuel.application.usecase.community.GetCityCommunitySummaryUseCase,
    private val communitySnapshot: com.anpfuel.domain.community.CommunityFeedLocalSnapshot,
) : ViewModel() {

    private val _uiState = MutableStateFlow(
        HomeUiState(
            hasVehicle = homeVehiclePresenceStore.hasVehicle(),
            showBenefits = homeBenefitsVisibilityStore.isVisible(),
        ),
    )
    val uiState: StateFlow<HomeUiState> = _uiState.asStateFlow()

    private var hasPresentedContent = false
    private var loadJob: kotlinx.coroutines.Job? = null
    private var loadGeneration = 0L
    private var communityJob: kotlinx.coroutines.Job? = null
    private var communityGeneration = 0L
    private var communitySession = 0L
    private var consecutiveCommunityFailures = 0
    private var lastCommunityAttemptMillis: Long? = null
    internal var communityNow: () -> java.time.Instant = { java.time.Instant.now() }
    /** A resumed screen owns a lease; cleanup from an older screen cannot cancel this one. */
    fun startCommunitySession(): Long {
        _uiState.update { it.copy(communityPrices = emptyList(), communityGeneratedAt = null, communityUpdatedAt = null, communityExpiresAt = null) }
        return ++communitySession
    }

    fun selectPriceSource(source: HomePriceSource, locale: Locale) {
        if (source == _uiState.value.priceSource) return
        loadGeneration++
        loadJob?.cancel()
        stopCommunityRefresh()
        _uiState.update { it.copy(priceSource = source, isLoading = false, communityPrices = emptyList(),
            communityGeneratedAt = null, communityUpdatedAt = null, communityExpiresAt = null) }
        if (source == HomePriceSource.ANP) load(locale)
    }

    fun stopCommunityRefresh(session: Long? = null) {
        if (session != null && session != communitySession) return
        communitySession++
        communityGeneration++
        communityJob?.cancel()
        _uiState.update { it.copy(communityLoading = false) }
    }

    internal fun expireCommunity(expectedExpiry: java.time.Instant) {
        _uiState.update { state ->
            if (state.priceSource == HomePriceSource.COMMUNITY && state.communityExpiresAt == expectedExpiry)
                state.copy(communityPrices = emptyList(), communityGeneratedAt = null,
                    communityUpdatedAt = null, communityExpiresAt = null)
            else state
        }
    }

    /**
     * Central policy delay for the Home community loop: zero only when a fetch
     * is due now, otherwise the time until the next event that could make one
     * due, never below the loop floor (a zero delay would busy-loop).
     */
    fun communityAutoRefreshDelayMillis(): Long {
        if (shouldFetchCommunity(explicit = false)) return 0L
        val nowMillis = communityNow().toEpochMilli()
        var delay = com.anpfuel.domain.community.CommunityRefreshPolicy.SWEEP_INTERVAL_MILLIS
        _uiState.value.communityGeneratedAt?.let { generated ->
            val remaining = com.anpfuel.domain.community.CommunityRefreshPolicy.STALE_AFTER_MILLIS - (nowMillis - generated.toEpochMilli())
            if (remaining > 0) delay = minOf(delay, remaining)
        }
        if (consecutiveCommunityFailures > 0 && com.anpfuel.domain.community.CommunityRefreshPolicy.shouldKeepRetrying(consecutiveCommunityFailures)) {
            val lastAttempt = lastCommunityAttemptMillis ?: nowMillis
            val remaining = com.anpfuel.domain.community.CommunityRefreshPolicy.retryDelayMillis(consecutiveCommunityFailures) - (nowMillis - lastAttempt)
            if (remaining > 0) delay = minOf(delay, remaining)
        }
        return maxOf(delay, com.anpfuel.domain.community.CommunityRefreshPolicy.MIN_LOOP_DELAY_MILLIS)
    }

    fun refreshCommunity(periodic: Boolean = false) {
        if (_uiState.value.priceSource != HomePriceSource.COMMUNITY) return
        // A periodic tick must not restart a slow city lookup plus HTTP load.
        // Explicit refreshes still replace it so city changes and retries win.
        if (periodic && communityJob?.isActive == true) return
        if (!periodic) consecutiveCommunityFailures = 0
        val ticket = ++communityGeneration
        communityJob?.cancel()
        _uiState.update { state ->
            state.copy(communityLoading = true, communityFailed = false,
                communityPrices = if (state.communityExpiresAt?.let { it > communityNow() } == true) state.communityPrices else emptyList())
        }
        communityJob = viewModelScope.launch {
            try {
                val city = communitySummary.city()
                if (ticket != communityGeneration) return@launch
                if (city != _uiState.value.communityCity) {
                    consecutiveCommunityFailures = 0
                    lastCommunityAttemptMillis = null
                    _uiState.update { it.copy(communityPrices = emptyList(), communityGeneratedAt = null,
                        communityUpdatedAt = null, communityExpiresAt = null, communityStale = false, communityCity = city) }
                }
                _uiState.update { it.copy(hasLocation = city != null, municipality = city?.name, state = city?.state) }
                if (city == null) {
                    _uiState.update { it.copy(communityLoading = false) }
                    return@launch
                }
                paintCommunitySnapshot(city, ticket)
                if (ticket != communityGeneration || _uiState.value.priceSource != HomePriceSource.COMMUNITY) return@launch
                if (!shouldFetchCommunity(!periodic)) {
                    if (ticket == communityGeneration) {
                        _uiState.update {
                            it.copy(
                                communityLoading = false,
                                // Explicit refreshes report offline; periodic ticks stay quiet.
                                communityFailed = if (!periodic && it.isOffline) true else it.communityFailed,
                            )
                        }
                    }
                    refreshCommunityStale()
                    return@launch
                }
                lastCommunityAttemptMillis = communityNow().toEpochMilli()
                val summary = communitySummary(city)
                if (ticket != communityGeneration || _uiState.value.priceSource != HomePriceSource.COMMUNITY) return@launch
                require(summary.city == city)
                val now = communityNow()
                consecutiveCommunityFailures = 0
                val live = summary.items.filter { it.expiresAt > now }
                val prices = live.sortedBy { it.fuel.ordinal }.map { price ->
                    AveragePriceUiModel(price.fuel,
                        com.anpfuel.app.community.CommunityPriceDisplay.formatMilliBrl(price.averageMilliBrl),
                        com.anpfuel.app.community.CommunityPriceDisplay.formatMilliBrl(price.minimumMilliBrl),
                        com.anpfuel.app.community.CommunityPriceDisplay.formatMilliBrl(price.maximumMilliBrl), price.stationCount)
                }
                _uiState.update { it.copy(communityLoading = false, communityFailed = false, communityStale = false, communityPrices = prices,
                    communityGeneratedAt = summary.generatedAt, communityUpdatedAt = live.maxOfOrNull { row -> row.updatedAt },
                    communityExpiresAt = live.minOfOrNull { row -> row.expiresAt }) }
            } catch (cancelled: kotlinx.coroutines.CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                if (ticket == communityGeneration) {
                    consecutiveCommunityFailures++
                    val current = _uiState.value
                    val retains = current.communityPrices.isNotEmpty() &&
                        (current.communityExpiresAt == null || communityNow().isBefore(current.communityExpiresAt))
                    _uiState.update {
                        it.copy(communityLoading = false, communityFailed = true,
                            communityStale = true,
                            communityPrices = if (retains) it.communityPrices else emptyList(),
                            communityGeneratedAt = if (retains) it.communityGeneratedAt else null,
                            communityUpdatedAt = if (retains) it.communityUpdatedAt else null,
                            communityExpiresAt = if (retains) it.communityExpiresAt else null)
                    }
                }
            }
        }
    }

    private suspend fun paintCommunitySnapshot(city: com.anpfuel.domain.community.FeedCity, ticket: Long) {
        val current = _uiState.value
        if (current.communityPrices.isNotEmpty() || current.communityGeneratedAt != null) return
        val snapshot = try {
            communitySnapshot.snapshotSummary(city)
        } catch (cancelled: kotlinx.coroutines.CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            null
        } ?: return
        if (ticket != communityGeneration) return
        val now = communityNow()
        val live = snapshot.items.filter { it.expiresAt > now }
        val prices = live.sortedBy { it.fuel.ordinal }.map { price ->
            AveragePriceUiModel(price.fuel,
                com.anpfuel.app.community.CommunityPriceDisplay.formatMilliBrl(price.averageMilliBrl),
                com.anpfuel.app.community.CommunityPriceDisplay.formatMilliBrl(price.minimumMilliBrl),
                com.anpfuel.app.community.CommunityPriceDisplay.formatMilliBrl(price.maximumMilliBrl), price.stationCount)
        }
        _uiState.update {
            it.copy(communityLoading = false, communityFailed = false, communityPrices = prices,
                communityGeneratedAt = snapshot.generatedAt,
                communityUpdatedAt = live.maxOfOrNull { row -> row.updatedAt },
                communityExpiresAt = live.minOfOrNull { row -> row.expiresAt })
        }
        refreshCommunityStale()
    }

    private fun shouldFetchCommunity(explicit: Boolean): Boolean {
        if (_uiState.value.priceSource != HomePriceSource.COMMUNITY) return false
        // No query without a connection; an aged snapshot stays with an error mark.
        if (_uiState.value.isOffline) return false
        // Never refetch within the loop floor of the previous attempt, except for
        // explicit refreshes; the loop delay already guarantees progress.
        val lastAttempt = lastCommunityAttemptMillis
        if (!explicit && lastAttempt != null &&
            communityNow().toEpochMilli() - lastAttempt < com.anpfuel.domain.community.CommunityRefreshPolicy.MIN_LOOP_DELAY_MILLIS
        ) return false
        if (!explicit) {
            if (consecutiveCommunityFailures > 0) {
                if (!com.anpfuel.domain.community.CommunityRefreshPolicy.shouldKeepRetrying(consecutiveCommunityFailures)) return false
                val lastAttempt = lastCommunityAttemptMillis ?: return true
                if (communityNow().toEpochMilli() - lastAttempt <
                    com.anpfuel.domain.community.CommunityRefreshPolicy.retryDelayMillis(consecutiveCommunityFailures)
                ) return false
            }
            val state = _uiState.value
            if (state.communityPrices.isEmpty() && state.communityGeneratedAt == null) return true
            return com.anpfuel.domain.community.CommunityRefreshPolicy.isStale(
                communityNow().toEpochMilli(), state.communityGeneratedAt?.toEpochMilli(),
            )
        }
        return true
    }

    private fun refreshCommunityStale() {
        val nowMillis = communityNow().toEpochMilli()
        _uiState.update { state ->
            state.copy(
                communityStale = state.communityGeneratedAt?.let { generated ->
                    com.anpfuel.domain.community.CommunityRefreshPolicy.isStale(nowMillis, generated.toEpochMilli())
                } ?: false,
            )
        }
    }

    init {
        viewModelScope.launch {
            var wasOffline = false
            observeNetworkConnectivityUseCase().collect { isConnected ->
                _uiState.update { it.copy(isOffline = !isConnected) }
                // A new network is an explicit event: reset backoff and re-evaluate.
                if (wasOffline && isConnected && _uiState.value.priceSource == HomePriceSource.COMMUNITY) {
                    consecutiveCommunityFailures = 0
                    refreshCommunity(periodic = true)
                }
                wasOffline = !isConnected
            }
        }
    }

    fun load(locale: Locale, showLoadingIndicator: Boolean = false) {
        if(_uiState.value.priceSource==HomePriceSource.COMMUNITY) {refreshCommunity();return}
        val ticket=++loadGeneration;loadJob?.cancel()
        loadJob=viewModelScope.launch {
            val showLoading = showLoadingIndicator || !hasPresentedContent
            if (showLoading) {
                _uiState.update { it.copy(isLoading = true, error = null, errorMessage = null) }
            } else {
                _uiState.update { it.copy(error = null, errorMessage = null) }
            }
            runCatching {
                val readinessResult = getDataReadinessUseCase()
                val preferred = selectLocationUseCase.getPreferredLocation()
                val vehicles = listVehiclesUseCase()
                if(ticket!=loadGeneration) return@launch

                if (preferred == null || !readinessResult.hasCachedData) {
                    homeVehiclePresenceStore.setHasVehicle(vehicles.isNotEmpty())
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            readiness = readinessResult.readiness,
                            hasCachedData = readinessResult.hasCachedData,
                            hasLocation = preferred != null,
                            municipality = preferred?.municipality,
                            state = preferred?.state,
                            surveyWeek = readinessResult.latestSurveyWeek,
                            prices = emptyList(),
                            tankFillCostEstimates = emptyList(),
                            isEmptyMunicipality = false,
                            dataAvailability = null,
                            operationalNote = null,
                            hasVehicle = vehicles.isNotEmpty(),
                        )
                    }
                    hasPresentedContent = true
                    return@launch
                }

                val pricesResult = getMunicipalityPricesUseCase()
                val tankFillResult = getTankFillCostEstimatesUseCase(
                    vehicles = vehicles,
                    state = pricesResult.state,
                    municipality = pricesResult.municipality,
                    surveyWeek = pricesResult.surveyWeek,
                )
                val uiPrices = com.anpfuel.app.mapper.AveragePriceUiMapper.toUiModels(
                    prices = pricesResult.prices,
                    locale = locale,
                )
                val uiTankFillEstimates = TankFillCostUiMapper.toUiModels(
                    items = tankFillResult.items,
                    locale = locale,
                    state = tankFillResult.state,
                    municipality = tankFillResult.municipality,
                )

                if(ticket!=loadGeneration) return@launch
                homeVehiclePresenceStore.setHasVehicle(vehicles.isNotEmpty())
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        readiness = readinessResult.readiness,
                        hasCachedData = readinessResult.hasCachedData,
                        hasLocation = true,
                        municipality = pricesResult.municipality,
                        state = pricesResult.state,
                        surveyWeek = pricesResult.surveyWeek,
                        prices = uiPrices,
                        tankFillCostEstimates = uiTankFillEstimates,
                        isEmptyMunicipality = pricesResult.isEmpty,
                        dataAvailability = pricesResult.dataAvailability,
                        operationalNote = pricesResult.operationalNote,
                        hasVehicle = vehicles.isNotEmpty(),
                    )
                }
                hasPresentedContent = true
            }.onFailure { error ->
                if(error is kotlinx.coroutines.CancellationException) throw error
                if(ticket!=loadGeneration) return@launch
                val appError = AppErrorResolver.fromThrowable(error)
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        error = appError,
                        errorMessage = error.message ?: error.javaClass.simpleName,
                    )
                }
                hasPresentedContent = true
            }
        }
    }

    fun onPriceSortChanged(order: HomePriceSortOrder) {
        _uiState.update { it.copy(priceSort = order) }
    }

    fun dismissBenefits() {
        homeBenefitsVisibilityStore.setVisible(false)
        _uiState.update { it.copy(showBenefits = false) }
    }

    fun refresh(locale: Locale) {
        if(_uiState.value.priceSource==HomePriceSource.COMMUNITY) {refreshCommunity();return}
        if (_uiState.value.isRefreshing) {
            return
        }

        viewModelScope.launch {
            _uiState.update { it.copy(isRefreshing = true, error = null, errorMessage = null) }
            runCatching {
                syncPriceTablesUseCase(SyncRequestSource.MANUAL)
            }.onFailure { error ->
                if (error !is DomainException) {
                    val appError = AppErrorResolver.fromThrowable(error)
                    _uiState.update {
                        it.copy(
                            isRefreshing = false,
                            error = appError,
                            errorMessage = error.message,
                        )
                    }
                    return@launch
                }
            }
            _uiState.update { it.copy(isRefreshing = false) }
            load(locale)
        }
    }
}
