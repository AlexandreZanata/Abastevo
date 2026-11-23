package com.anpfuel.app.ui.premium

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.premium.PurchaseEligibility
import com.anpfuel.domain.premium.StorePlan

/**
 * Google Play purchase section (B-BR-STORE-01/10). It renders only for a
 * server-listed Play offer: plans with store-formatted prices, a pending
 * payment, or Premium already active through another channel. No web price,
 * checkout or link is ever shown.
 */
@Composable
internal fun PremiumStoreSection(
    state: PremiumUiState,
    onSubscribe: (StorePlan) -> Unit,
    onRetryPlans: () -> Unit,
) {
    when {
        state.eligibility == PurchaseEligibility.ACTIVE_FROM_OTHER_SOURCE -> {
            PremiumSectionTitle(stringResource(R.string.premium_store_section_title))
            PremiumMessageCard(stringResource(R.string.premium_active_other_source))
        }
        state.eligibility == PurchaseEligibility.PENDING ||
            (state.eligibility == PurchaseEligibility.AVAILABLE && state.storePending) -> {
            PremiumSectionTitle(stringResource(R.string.premium_store_section_title))
            PremiumMessageCard(stringResource(R.string.premium_purchase_pending))
        }
        state.eligibility == PurchaseEligibility.AVAILABLE -> {
            PremiumSectionTitle(stringResource(R.string.premium_store_section_title))
            PremiumPlansCard(state = state, onSubscribe = onSubscribe, onRetryPlans = onRetryPlans)
        }
        else -> Unit
    }
}

/** Store or server step in progress, shown above the status. */
@Composable
internal fun PremiumPurchaseProgress(phase: PremiumPurchasePhase) {
    val label = premiumPurchasePhaseRes(phase) ?: return
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        LinearProgressIndicator(Modifier.fillMaxWidth())
        Text(
            text = stringResource(label),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun PremiumMessageCard(message: String, modifier: Modifier = Modifier) {
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Text(
            text = message,
            modifier = Modifier.padding(16.dp),
            style = MaterialTheme.typography.bodyMedium,
        )
    }
}

@Composable
private fun PremiumPlansCard(
    state: PremiumUiState,
    onSubscribe: (StorePlan) -> Unit,
    onRetryPlans: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            when {
                state.plansLoading -> {
                    LinearProgressIndicator(Modifier.fillMaxWidth())
                    Text(
                        text = stringResource(R.string.premium_store_plans_loading),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                state.plans.isEmpty() -> {
                    Text(
                        text = stringResource(R.string.premium_store_plans_unavailable),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    TextButton(onClick = onRetryPlans) {
                        Text(stringResource(R.string.action_retry))
                    }
                }
                else -> {
                    state.plans.forEachIndexed { index, plan ->
                        if (index > 0) HorizontalDivider()
                        PremiumPlanRow(
                            plan = plan,
                            enabled = !state.purchaseBusy && !state.loading,
                            onSubscribe = { onSubscribe(plan) },
                        )
                    }
                }
            }
            Text(
                text = stringResource(R.string.premium_store_billing_note),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun PremiumPlanRow(plan: StorePlan, enabled: Boolean, onSubscribe: () -> Unit) {
    val title = premiumPlanTitle(plan.billingPeriod)
    val price = premiumPlanPrice(plan.billingPeriod)
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(
                text = if (title.count == null) stringResource(title.res) else stringResource(title.res, title.count),
                style = MaterialTheme.typography.titleSmall,
            )
            Text(
                text = if (price.count == null) {
                    stringResource(price.res, plan.formattedPrice)
                } else {
                    stringResource(price.res, plan.formattedPrice, price.count)
                },
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Button(onClick = onSubscribe, enabled = enabled) {
            Text(stringResource(R.string.premium_store_subscribe))
        }
    }
}
