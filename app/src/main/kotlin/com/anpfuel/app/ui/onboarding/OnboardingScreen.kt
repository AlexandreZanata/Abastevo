package com.anpfuel.app.ui.onboarding

import android.Manifest
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.accessibility.headingSemantics
import com.anpfuel.app.ui.components.AnpAttributionFooter
import com.anpfuel.app.ui.components.GeocodingAttributionFooter
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.app.ui.weekpicker.WeekPickerConfirmDialog
import com.anpfuel.app.ui.weekpicker.WeekPickerContent
import com.anpfuel.app.ui.weekpicker.WeekPickerUiState
import com.anpfuel.domain.model.SurveyWeekCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeek
import java.time.LocalDate
import kotlinx.coroutines.flow.distinctUntilChanged

@Composable
fun OnboardingScreen(
    onNavigateToHome: () -> Unit,
    onNavigateToLocation: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: OnboardingViewModel = hiltViewModel(),
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]

    val permissionLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestMultiplePermissions(),
    ) { permissions ->
        val granted = permissions[Manifest.permission.ACCESS_FINE_LOCATION] == true ||
            permissions[Manifest.permission.ACCESS_COARSE_LOCATION] == true
        if (granted) {
            viewModel.onLocationPermissionGranted()
        } else {
            viewModel.onLocationPermissionDenied()
        }
    }

    LaunchedEffect(viewModel) {
        viewModel.locationPermissionRequest.collect {
            permissionLauncher.launch(
                arrayOf(
                    Manifest.permission.ACCESS_FINE_LOCATION,
                    Manifest.permission.ACCESS_COARSE_LOCATION,
                ),
            )
        }
    }

    LaunchedEffect(viewModel) {
        viewModel.navigation.collect { destination ->
            when (destination) {
                OnboardingNavigation.ToHome -> onNavigateToHome()
                OnboardingNavigation.ToLocation -> onNavigateToLocation()
            }
        }
    }

    OnboardingContent(
        uiState = uiState,
        onNextPage = viewModel::onNextPage,
        onPreviousPage = viewModel::onPreviousPage,
        onPageSelected = viewModel::onPageSelected,
        onProceedToWeekPicker = viewModel::proceedToWeekPicker,
        onUseLatestWeekAndSync = viewModel::useLatestWeekAndSync,
        onSelectWeek = viewModel::selectWeekAndSync,
        onRetryCatalog = viewModel::retryCatalogDiscovery,
        onRetrySync = viewModel::retrySync,
        onSkipSync = viewModel::skipSync,
        onBackToIntro = viewModel::backToIntro,
        onUseDeviceLocation = viewModel::onUseDeviceLocationClick,
        onChooseManualLocation = viewModel::onChooseManualLocation,
        modifier = modifier,
    )

    uiState.pendingConfirmation?.let { entry ->
        WeekPickerConfirmDialog(
            entry = entry,
            locale = locale,
            onConfirm = viewModel::confirmPendingWeek,
            onDismiss = viewModel::dismissPendingConfirmation,
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun OnboardingContent(
    uiState: OnboardingUiState,
    onNextPage: () -> Unit,
    onPreviousPage: () -> Unit,
    onPageSelected: (Int) -> Unit,
    onProceedToWeekPicker: () -> Unit,
    onUseLatestWeekAndSync: () -> Unit,
    onSelectWeek: (SurveyWeekCatalogEntry) -> Unit,
    onRetryCatalog: () -> Unit,
    onRetrySync: () -> Unit,
    onSkipSync: () -> Unit,
    onBackToIntro: () -> Unit,
    onUseDeviceLocation: () -> Unit,
    onChooseManualLocation: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val titleRes = when (uiState.step) {
        OnboardingStep.INTRO -> R.string.onboarding_title_welcome
        OnboardingStep.LOCATION_PROMPT -> R.string.onboarding_location_prompt_title
        OnboardingStep.WEEK_PICKER, OnboardingStep.SYNCING -> R.string.week_picker_title
    }

    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(text = stringResource(titleRes)) },
                navigationIcon = {
                    if (uiState.step == OnboardingStep.WEEK_PICKER && !uiState.isLoadingCatalog) {
                        TextButton(onClick = onBackToIntro) {
                            Text(text = stringResource(R.string.action_back))
                        }
                    }
                },
            )
        },
        bottomBar = {
            when (uiState.step) {
                OnboardingStep.LOCATION_PROMPT -> GeocodingAttributionFooter()
                else -> AnpAttributionFooter()
            }
        },
    ) { innerPadding ->
        when (uiState.step) {
            OnboardingStep.SYNCING -> {
                LoadingState(
                    message = stringResource(R.string.sync_progress_discovering),
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(innerPadding),
                )
            }

            OnboardingStep.WEEK_PICKER -> {
                WeekPickerContent(
                    uiState = WeekPickerUiState(
                        catalog = uiState.catalog,
                        isLoadingCatalog = uiState.isLoadingCatalog,
                        catalogError = uiState.catalogError,
                        syncError = uiState.error,
                        showSkipAction = true,
                    ),
                    onUseLatestWeek = onUseLatestWeekAndSync,
                    onSelectWeek = onSelectWeek,
                    onRetryCatalog = onRetryCatalog,
                    onRetrySync = onRetrySync,
                    onSkip = onSkipSync,
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(innerPadding),
                )
            }

            OnboardingStep.INTRO -> {
                OnboardingIntroContent(
                    uiState = uiState,
                    onNextPage = onNextPage,
                    onPreviousPage = onPreviousPage,
                    onPageSelected = onPageSelected,
                    onProceedToWeekPicker = onProceedToWeekPicker,
                    onSkipSync = onSkipSync,
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(innerPadding),
                )
            }

            OnboardingStep.LOCATION_PROMPT -> {
                LocationPromptStep(
                    isResolvingLocation = uiState.isResolvingLocation,
                    onUseDeviceLocation = onUseDeviceLocation,
                    onChooseManualLocation = onChooseManualLocation,
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(innerPadding),
                )
            }
        }
    }
}

@Composable
private fun OnboardingIntroContent(
    uiState: OnboardingUiState,
    onNextPage: () -> Unit,
    onPreviousPage: () -> Unit,
    onPageSelected: (Int) -> Unit,
    onProceedToWeekPicker: () -> Unit,
    onSkipSync: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val pagerState = rememberPagerState(
        initialPage = uiState.pageIndex,
        pageCount = { uiState.pageCount },
    )

    LaunchedEffect(uiState.pageIndex) {
        if (pagerState.currentPage != uiState.pageIndex) {
            pagerState.animateScrollToPage(uiState.pageIndex)
        }
    }

    LaunchedEffect(pagerState) {
        snapshotFlow { pagerState.currentPage }
            .distinctUntilChanged()
            .collect(onPageSelected)
    }

    Column(
        modifier = modifier
            .padding(horizontal = 24.dp, vertical = 16.dp),
        verticalArrangement = Arrangement.SpaceBetween,
    ) {
        HorizontalPager(
            state = pagerState,
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth(),
        ) { page ->
            OnboardingPageContent(pageIndex = page)
        }

        OnboardingIntroActions(
            uiState = uiState,
            onNextPage = onNextPage,
            onPreviousPage = onPreviousPage,
            onProceedToWeekPicker = onProceedToWeekPicker,
            onSkipSync = onSkipSync,
        )
    }
}

@Composable
private fun OnboardingPageContent(pageIndex: Int) {
    val (titleRes, bodyRes) = when (pageIndex) {
        0 -> R.string.onboarding_title_welcome to R.string.onboarding_body_anp_source
        1 -> R.string.onboarding_title_offline to R.string.onboarding_body_offline
        else -> R.string.onboarding_title_ready to R.string.onboarding_body_ready
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = stringResource(titleRes),
            style = MaterialTheme.typography.headlineMedium,
            color = MaterialTheme.colorScheme.onBackground,
            textAlign = TextAlign.Center,
            modifier = Modifier.headingSemantics(),
        )
        Text(
            text = stringResource(bodyRes),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(top = 16.dp),
        )
    }
}

@Composable
private fun OnboardingIntroActions(
    uiState: OnboardingUiState,
    onNextPage: () -> Unit,
    onPreviousPage: () -> Unit,
    onProceedToWeekPicker: () -> Unit,
    onSkipSync: () -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        if (uiState.isOnLastPage) {
            Button(
                onClick = onProceedToWeekPicker,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(text = stringResource(R.string.onboarding_action_get_started))
            }
            TextButton(
                onClick = onSkipSync,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(text = stringResource(R.string.onboarding_action_skip))
            }
        } else {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                if (uiState.pageIndex > 0) {
                    TextButton(onClick = onPreviousPage) {
                        Text(text = stringResource(R.string.action_back))
                    }
                }
                Button(
                    onClick = onNextPage,
                    modifier = Modifier.weight(1f),
                ) {
                    Text(text = stringResource(R.string.action_next))
                }
            }
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun OnboardingIntroContentPreview() {
    AnpFuelTheme {
        OnboardingContent(
            uiState = OnboardingUiState(pageIndex = 2),
            onNextPage = {},
            onPreviousPage = {},
            onPageSelected = {},
            onProceedToWeekPicker = {},
            onUseLatestWeekAndSync = {},
            onSelectWeek = {},
            onRetryCatalog = {},
            onRetrySync = {},
            onSkipSync = {},
            onBackToIntro = {},
            onUseDeviceLocation = {},
            onChooseManualLocation = {},
        )
    }
}

@Preview(showBackground = true)
@Composable
private fun OnboardingWeekPickerPreview() {
    val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    AnpFuelTheme {
        OnboardingContent(
            uiState = OnboardingUiState(
                step = OnboardingStep.WEEK_PICKER,
                catalog = listOf(
                    SurveyWeekCatalogEntry.create(
                        surveyWeek = week,
                        summaryUrl = "https://example.com/summary.xlsx",
                        stationUrl = "https://example.com/station.xlsx",
                        publishedAt = LocalDate.parse("2026-06-12"),
                    ),
                ),
            ),
            onNextPage = {},
            onPreviousPage = {},
            onPageSelected = {},
            onProceedToWeekPicker = {},
            onUseLatestWeekAndSync = {},
            onSelectWeek = {},
            onRetryCatalog = {},
            onRetrySync = {},
            onSkipSync = {},
            onBackToIntro = {},
            onUseDeviceLocation = {},
            onChooseManualLocation = {},
        )
    }
}

@Preview(showBackground = true)
@Composable
private fun OnboardingLocationPromptPreview() {
    AnpFuelTheme {
        OnboardingContent(
            uiState = OnboardingUiState(step = OnboardingStep.LOCATION_PROMPT),
            onNextPage = {},
            onPreviousPage = {},
            onPageSelected = {},
            onProceedToWeekPicker = {},
            onUseLatestWeekAndSync = {},
            onSelectWeek = {},
            onRetryCatalog = {},
            onRetrySync = {},
            onSkipSync = {},
            onBackToIntro = {},
            onUseDeviceLocation = {},
            onChooseManualLocation = {},
        )
    }
}
