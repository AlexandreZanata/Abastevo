package com.anpfuel.app.ui.premium

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material.icons.filled.Storefront
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.ProfileActionRow
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.app.ui.theme.PremiumColors
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PremiumManagementScreen(
    onBack: () -> Unit,
    onSignIn: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: PremiumViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val uriHandler = LocalUriHandler.current
    val openStore = {
        if (runCatching { uriHandler.openUri(PremiumStatus.MANAGEMENT_URL) }.isFailure) {
            viewModel.storeOpenFailed()
        } else {
            viewModel.invalidateCache()
        }
    }
    val lifecycleOwner = LocalLifecycleOwner.current
    LaunchedEffect(viewModel, lifecycleOwner) {
        lifecycleOwner.lifecycle.repeatOnLifecycle(Lifecycle.State.RESUMED) {
            viewModel.refresh()
        }
    }
    PremiumManagementContent(
        state = state,
        onBack = onBack,
        onSignIn = onSignIn,
        onOpenStore = openStore,
        onRefresh = viewModel::forceRefresh,
        onClearNotice = viewModel::clearNotice,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun PremiumManagementContent(
    state: PremiumUiState,
    onBack: () -> Unit,
    onSignIn: () -> Unit,
    onOpenStore: () -> Unit,
    onRefresh: () -> Unit,
    onClearNotice: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.premium_manage)) },
                onNavigateUp = onBack,
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            when {
                state.loading && state.status == null -> {
                    PremiumManagementSkeletonSections(
                        state = state,
                        onOpenStore = onOpenStore,
                        onRefresh = onRefresh,
                    )
                }
                state.signedOut -> {
                    PremiumSignedOutCard(onSignIn = onSignIn)
                }
                else -> {
                    if (state.loading) {
                        LinearProgressIndicator(Modifier.fillMaxWidth())
                    }
                    PremiumSectionTitle(stringResource(R.string.premium_status_section_title))
                    PremiumStatusCard(status = state.status)
                    state.notice?.let { notice ->
                        PremiumNoticeCard(
                            notice = notice,
                            onRetry = if (notice == PremiumNotice.STORE_FAILED) onOpenStore else onRefresh,
                            onSignIn = onSignIn,
                            onDismiss = onClearNotice,
                        )
                    }
                    PremiumManagementGuidanceCard()
                    PremiumManagementActionsCard(
                        onOpenStore = onOpenStore,
                        onRefresh = onRefresh,
                        refreshEnabled = !state.loading,
                    )
                }
            }
        }
    }
}

@Composable
private fun PremiumManagementSkeletonSections(
    state: PremiumUiState,
    onOpenStore: () -> Unit,
    onRefresh: () -> Unit,
) {
    Text(
        text = stringResource(R.string.premium_loading),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    PremiumSectionTitle(stringResource(R.string.premium_status_section_title))
    PremiumStatusSkeletonCard()
    PremiumManagementGuidanceCard()
    PremiumManagementActionsCard(
        onOpenStore = onOpenStore,
        onRefresh = onRefresh,
        refreshEnabled = !state.loading,
    )
}

@Composable
private fun PremiumManagementGuidanceCard(modifier: Modifier = Modifier) {
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = stringResource(R.string.premium_management_guidance),
                style = MaterialTheme.typography.bodyMedium,
            )
            Text(
                text = stringResource(R.string.premium_management_refresh_hint),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun PremiumManagementActionsCard(
    onOpenStore: () -> Unit,
    onRefresh: () -> Unit,
    refreshEnabled: Boolean,
    modifier: Modifier = Modifier,
) {
    val palette = PremiumColors.current()
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column {
            ProfileActionRow(
                icon = Icons.Default.Storefront,
                title = stringResource(R.string.premium_manage_in_store),
                onClick = onOpenStore,
                iconTint = palette.accent,
                trailingIcon = Icons.AutoMirrored.Filled.OpenInNew,
            )
            HorizontalDivider()
            ProfileActionRow(
                icon = Icons.Default.Refresh,
                title = stringResource(R.string.premium_refresh_status),
                onClick = onRefresh,
                enabled = refreshEnabled,
                iconTint = palette.accent,
            )
        }
    }
}

@Preview(showBackground = true, name = "Premium management light")
@Composable
private fun PremiumManagementLightPreview() {
    AnpFuelTheme(darkTheme = false, dynamicColor = false) {
        PremiumManagementContent(
            state = PremiumUiState(
                signedOut = false,
                status = PremiumStatus(
                    policyVersion = 1,
                    state = PremiumState.CANCELED,
                    reason = "verified",
                    capabilities = setOf("cloud.sync"),
                    validUntil = "2026-11-30T23:59:59Z",
                    retainedDataAccess = true,
                    paymentsConfigured = false,
                    managementUrl = PremiumStatus.MANAGEMENT_URL,
                ),
            ),
            onBack = {},
            onSignIn = {},
            onOpenStore = {},
            onRefresh = {},
            onClearNotice = {},
        )
    }
}

@Preview(showBackground = true, name = "Premium management dark")
@Composable
private fun PremiumManagementDarkPreview() {
    AnpFuelTheme(darkTheme = true, dynamicColor = false) {
        PremiumManagementContent(
            state = PremiumUiState(
                signedOut = false,
                status = PremiumStatus(
                    policyVersion = 1,
                    state = PremiumState.PAUSED,
                    reason = "verified",
                    capabilities = emptySet(),
                    validUntil = "2026-11-30T23:59:59Z",
                    retainedDataAccess = true,
                    paymentsConfigured = false,
                    managementUrl = PremiumStatus.MANAGEMENT_URL,
                ),
            ),
            onBack = {},
            onSignIn = {},
            onOpenStore = {},
            onRefresh = {},
            onClearNotice = {},
        )
    }
}
