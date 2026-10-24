package com.anpfuel.app.ui.premium

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.CreditCard
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Restore
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
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
fun PremiumScreen(
    onBack: () -> Unit,
    onSignIn: () -> Unit,
    onManageSubscription: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: PremiumViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val lifecycleOwner = LocalLifecycleOwner.current
    LaunchedEffect(viewModel, lifecycleOwner) {
        lifecycleOwner.lifecycle.repeatOnLifecycle(Lifecycle.State.RESUMED) {
            viewModel.refresh()
        }
    }
    PremiumContent(
        state = state,
        onBack = onBack,
        onSignIn = onSignIn,
        onManageSubscription = onManageSubscription,
        onRefresh = viewModel::refresh,
        onRestoreUnavailable = viewModel::restoreUnavailable,
        onClearNotice = viewModel::clearNotice,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun PremiumContent(
    state: PremiumUiState,
    onBack: () -> Unit,
    onSignIn: () -> Unit,
    onManageSubscription: () -> Unit,
    onRefresh: () -> Unit,
    onRestoreUnavailable: () -> Unit,
    onClearNotice: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.premium_section_title)) },
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
                    PremiumLoadingCard()
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
                            onRetry = onRefresh,
                            onSignIn = onSignIn,
                            onDismiss = onClearNotice,
                        )
                    }
                    state.status?.let { status ->
                        PremiumSectionTitle(stringResource(R.string.premium_capabilities_title))
                        PremiumCapabilitiesCard(status = status)
                    }
                    PremiumSectionTitle(stringResource(R.string.premium_actions_section_title))
                    PremiumActionsCard(
                        state = state,
                        onRefresh = onRefresh,
                        onManageSubscription = onManageSubscription,
                        onRestoreUnavailable = onRestoreUnavailable,
                    )
                }
            }
        }
    }
}

@Composable
private fun PremiumCapabilitiesCard(status: PremiumStatus, modifier: Modifier = Modifier) {
    val palette = PremiumColors.current()
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    val capabilities = premiumCapabilityLabels(status.capabilities)
    Card(
        modifier = modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        if (capabilities.isEmpty()) {
            Text(
                text = stringResource(R.string.premium_no_capabilities),
                modifier = Modifier.padding(16.dp),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } else {
            Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                capabilities.forEach { labelRes ->
                    Row(
                        horizontalArrangement = Arrangement.spacedBy(10.dp),
                    ) {
                        Icon(
                            imageVector = Icons.Default.Check,
                            contentDescription = null,
                            tint = palette.accent,
                        )
                        Text(
                            text = stringResource(labelRes),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun PremiumActionsCard(
    state: PremiumUiState,
    onRefresh: () -> Unit,
    onManageSubscription: () -> Unit,
    onRestoreUnavailable: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = PremiumColors.current()
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column {
            ProfileActionRow(
                icon = Icons.Default.Refresh,
                title = stringResource(R.string.premium_refresh_status),
                onClick = onRefresh,
                enabled = !state.loading,
                iconTint = palette.accent,
            )
            HorizontalDivider()
            ProfileActionRow(
                icon = Icons.Default.CreditCard,
                title = stringResource(R.string.premium_manage),
                onClick = onManageSubscription,
                iconTint = palette.accent,
            )
            HorizontalDivider()
            ProfileActionRow(
                icon = Icons.Default.Restore,
                title = stringResource(R.string.premium_restore),
                subtitle = stringResource(R.string.premium_restore_note),
                onClick = onRestoreUnavailable,
                iconTint = palette.accent,
            )
        }
    }
}

private val previewStatus = PremiumStatus(
    policyVersion = 1,
    state = PremiumState.ACTIVE,
    reason = "verified",
    capabilities = setOf("vehicles.multiple", "cloud.sync"),
    validUntil = "2026-11-30T23:59:59Z",
    retainedDataAccess = true,
    paymentsConfigured = false,
    managementUrl = PremiumStatus.MANAGEMENT_URL,
)

@Preview(showBackground = true, name = "Premium light")
@Composable
private fun PremiumLightPreview() {
    AnpFuelTheme(darkTheme = false, dynamicColor = false) {
        PremiumContent(
            state = PremiumUiState(signedOut = false, status = previewStatus),
            onBack = {},
            onSignIn = {},
            onManageSubscription = {},
            onRefresh = {},
            onRestoreUnavailable = {},
            onClearNotice = {},
        )
    }
}

@Preview(showBackground = true, name = "Premium dark")
@Composable
private fun PremiumDarkPreview() {
    AnpFuelTheme(darkTheme = true, dynamicColor = false) {
        PremiumContent(
            state = PremiumUiState(signedOut = false, status = previewStatus),
            onBack = {},
            onSignIn = {},
            onManageSubscription = {},
            onRefresh = {},
            onRestoreUnavailable = {},
            onClearNotice = {},
        )
    }
}
