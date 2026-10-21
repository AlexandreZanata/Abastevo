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
import com.anpfuel.data.local.preferences.ManualSyncVerificationStore
import com.anpfuel.app.mapper.TankFillCostUiMapper
import com.anpfuel.app.ui.model.AveragePriceUiModel
import com.anpfuel.app.ui.model.TankFillCostEstimateUiModel
import com.anpfuel.domain.event.SyncJobOutcome
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
    val lastVerifiedAtMillis: Long? = null,
    val verifiedSurveyWeek: SurveyWeek? = null,
    val lastSyncOutcome: SyncJobOutcome? = null,
) {
    /**
     * True when STALE is already verified for the displayed week:
     * manual sync confirmed ANP has no newer week yet. Global per week,
     * so switching city keeps the same verified state.
     */
    val isStaleVerified: Boolean
        get() = readiness == DataReadinessState.STALE &&
            lastSyncOutcome == SyncJobOutcome.NO_NEW_DATA &&
            lastVerifiedAtMillis != null &&
            verifiedSurveyWeek != null &&
            verifiedSurveyWeek == surveyWeek
}

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
    private val manualSyncVerificationStore: ManualSyncVerificationStore,
    observeNetworkConnectivityUseCase: ObserveNetworkConnectivityUseCase,
    private val communitySummary: com.anpfuel.application.usecase.community.GetCityCommunitySummaryUseCase,
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

    fun refreshCommunity(periodic: Boolean = false) {
        if (_uiState.value.priceSource != HomePriceSource.COMMUNITY) return
        // A periodic tick must not restart a slow city lookup plus HTTP load.
        // Explicit refreshes still replace it so city changes and retries win.
        if (periodic && communityJob?.isActive == true) return
        val ticket = ++communityGeneration
        communityJob?.cancel()
        _uiState.update { state ->
            state.copy(communityLoading = true, communityFailed = false,
                communityPrices = if (state.communityExpiresAt?.let { it > java.time.Instant.now() } == true) state.communityPrices else emptyList())
        }
        communityJob = viewModelScope.launch {
            try {
                val city = communitySummary.city()
                if (ticket != communityGeneration) return@launch
                if (city != _uiState.value.communityCity) {
                    _uiState.update { it.copy(communityPrices = emptyList(), communityGeneratedAt = null,
                        communityUpdatedAt = null, communityExpiresAt = null, communityCity = city) }
                }
                _uiState.update { it.copy(hasLocation = city != null, municipality = city?.name, state = city?.state) }
                if (city == null) {
                    _uiState.update { it.copy(communityLoading = false) }
                    return@launch
                }
                val summary = communitySummary(city)
                if (ticket != communityGeneration || _uiState.value.priceSource != HomePriceSource.COMMUNITY) return@launch
                require(summary.city == city)
                val now = java.time.Instant.now()
                val live = summary.items.filter { it.expiresAt > now }
                val prices = live.sortedBy { it.fuel.ordinal }.map { price ->
                    AveragePriceUiModel(price.fuel,
                        com.anpfuel.app.community.CommunityPriceDisplay.formatMilliBrl(price.averageMilliBrl),
                        com.anpfuel.app.community.CommunityPriceDisplay.formatMilliBrl(price.minimumMilliBrl),
                        com.anpfuel.app.community.CommunityPriceDisplay.formatMilliBrl(price.maximumMilliBrl), price.stationCount)
                }
                _uiState.update { it.copy(communityLoading = false, communityFailed = false, communityPrices = prices,
                    communityGeneratedAt = summary.generatedAt, communityUpdatedAt = live.maxOfOrNull { row -> row.updatedAt },
                    communityExpiresAt = live.minOfOrNull { row -> row.expiresAt }) }
            } catch (cancelled: kotlinx.coroutines.CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                if (ticket == communityGeneration) {
                    _uiState.update { it.copy(communityLoading = false, communityFailed = true,
                        communityPrices = emptyList(), communityGeneratedAt = null, communityUpdatedAt = null, communityExpiresAt = null) }
                }
            }
        }
    }

    init {
        viewModelScope.launch {
            observeNetworkConnectivityUseCase().collect { isConnected ->
                _uiState.update { it.copy(isOffline = !isConnected) }
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
                    val displayed = readinessResult.latestSurveyWeek
                    val verification = resolveVerification(displayed, readinessResult.latestSurveyWeek)
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            readiness = readinessResult.readiness,
                            hasCachedData = readinessResult.hasCachedData,
                            hasLocation = preferred != null,
                            municipality = preferred?.municipality,
                            state = preferred?.state,
                            surveyWeek = displayed,
                            prices = emptyList(),
                            tankFillCostEstimates = emptyList(),
                            isEmptyMunicipality = false,
                            dataAvailability = null,
                            operationalNote = null,
                            hasVehicle = vehicles.isNotEmpty(),
                            lastVerifiedAtMillis = verification.first,
                            verifiedSurveyWeek = verification.second,
                            lastSyncOutcome = verification.third,
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
                val verification = resolveVerification(
                    pricesResult.surveyWeek,
                    readinessResult.latestSurveyWeek,
                )
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
                        lastVerifiedAtMillis = verification.first,
                        verifiedSurveyWeek = verification.second,
                        lastSyncOutcome = verification.third,
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
            var outcome: SyncJobOutcome? = null
            runCatching {
                syncPriceTablesUseCase(SyncRequestSource.MANUAL)
            }.onSuccess { result ->
                outcome = result.outcome
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
            when (outcome) {
                SyncJobOutcome.NO_NEW_DATA -> {
                    _uiState.value.surveyWeek?.let { week ->
                        manualSyncVerificationStore.saveVerified(
                            weekStartIso = week.startDate.toString(),
                            weekEndIso = week.endDate.toString(),
                            verifiedAtMillis = System.currentTimeMillis(),
                        )
                    }
                }
                SyncJobOutcome.SUCCESS, SyncJobOutcome.PARTIAL -> {
                    manualSyncVerificationStore.clear()
                    _uiState.update {
                        it.copy(
                            lastVerifiedAtMillis = null,
                            verifiedSurveyWeek = null,
                            lastSyncOutcome = outcome,
                        )
                    }
                }
                else -> Unit
            }
            _uiState.update { it.copy(isRefreshing = false) }
            load(locale)
        }
    }

    /**
     * Verification is global per week (one sync covers every city).
     * Only present it when the displayed week is also the latest imported
     * week; a pinned old week must surface the week switch instead.
     */
    private fun resolveVerification(
        displayed: SurveyWeek?,
        latest: SurveyWeek?,
    ): Triple<Long?, SurveyWeek?, SyncJobOutcome?> {
        val stored = manualSyncVerificationStore.get() ?: return Triple(null, null, null)
        if (displayed == null || latest == null) return Triple(null, null, null)
        if (stored.weekStartIso != displayed.startDate.toString() ||
            stored.weekEndIso != displayed.endDate.toString()
        ) {
            return Triple(null, null, null)
        }
        if (displayed != latest) return Triple(null, null, null)
        if (stored.weekStartIso != latest.startDate.toString() ||
            stored.weekEndIso != latest.endDate.toString()
        ) {
            manualSyncVerificationStore.clear()
            return Triple(null, null, null)
        }
        return Triple(stored.verifiedAtMillis, displayed, SyncJobOutcome.NO_NEW_DATA)
    }
}
