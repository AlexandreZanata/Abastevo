package com.anpfuel.app.ui.premium

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.SkeletonLine
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.app.ui.theme.PremiumColors

@Composable
internal fun PremiumSignedOutCard(onSignIn: () -> Unit, modifier: Modifier = Modifier) {
    val palette = PremiumColors.current()
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(
            modifier = Modifier.padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(
                painter = painterResource(R.drawable.ic_premium_crown),
                contentDescription = null,
                tint = palette.accent,
                modifier = Modifier.size(32.dp),
            )
            Text(
                text = stringResource(R.string.premium_signed_out_title),
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.semantics { heading() },
            )
            Text(
                text = stringResource(R.string.premium_signed_out_message),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Button(onClick = onSignIn, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.profile_action_sign_in))
            }
        }
    }
}

@Composable
internal fun PremiumStatusSkeletonCard(modifier: Modifier = Modifier) {
    val palette = PremiumColors.current()
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Row(
            modifier = Modifier.padding(16.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(
                painter = painterResource(R.drawable.ic_premium_crown),
                contentDescription = null,
                tint = palette.accent,
                modifier = Modifier.size(24.dp),
            )
            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                SkeletonLine(width = 150.dp, height = 20.dp)
                Text(
                    text = stringResource(R.string.premium_valid_until, "—"),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    text = stringResource(R.string.premium_retained),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
internal fun PremiumCapabilitiesSkeletonCard(modifier: Modifier = Modifier) {
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            repeat(3) { index ->
                Row(
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(
                        imageVector = Icons.Default.Check,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.outlineVariant,
                    )
                    SkeletonLine(
                        width = if (index % 2 == 0) 190.dp else 150.dp,
                        height = 16.dp,
                    )
                }
            }
        }
    }
}

@Composable
internal fun PremiumNoticeCard(
    notice: PremiumNotice,
    onRetry: (() -> Unit)?,
    onSignIn: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    val action = when (notice) {
        PremiumNotice.DENIED -> onSignIn
        PremiumNotice.OFFLINE,
        PremiumNotice.ERROR,
        PremiumNotice.STORE_FAILED,
        -> onRetry
        PremiumNotice.RESTORE_UNAVAILABLE -> null
    }
    val actionLabel = if (notice == PremiumNotice.DENIED) {
        R.string.profile_action_sign_in
    } else {
        R.string.action_retry
    }
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text(
                text = stringResource(premiumNoticeMessageRes(notice)),
                style = MaterialTheme.typography.bodyMedium,
                color = if (notice == PremiumNotice.ERROR) {
                    MaterialTheme.colorScheme.error
                } else {
                    MaterialTheme.colorScheme.onSurface
                },
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (action != null) {
                    TextButton(onClick = action) {
                        Text(stringResource(actionLabel))
                    }
                }
                TextButton(onClick = onDismiss) {
                    Text(stringResource(R.string.action_dismiss))
                }
            }
        }
    }
}


@Composable
internal fun PremiumSectionTitle(title: String) {
    Text(
        text = title,
        style = MaterialTheme.typography.titleSmall,
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.semantics { heading() },
    )
}

@Composable
internal fun PremiumStatusCard(status: PremiumStatus?, modifier: Modifier = Modifier) {
    val palette = PremiumColors.current()
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Row(
            modifier = Modifier.padding(16.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(
                painter = painterResource(R.drawable.ic_premium_crown),
                contentDescription = null,
                tint = palette.accent,
                modifier = Modifier.size(24.dp),
            )
            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                Text(
                    text = stringResource(premiumStateLabelRes(status?.state)),
                    style = MaterialTheme.typography.titleMedium,
                )
                if (status != null && premiumShowsAccessUntil(status.state, status.validUntil)) {
                    Text(
                        text = stringResource(
                            R.string.premium_valid_until,
                            status.validUntil!!.take(10),
                        ),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (status != null && status.state != PremiumState.ACTIVE && status.retainedDataAccess) {
                    Text(
                        text = stringResource(R.string.premium_retained),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}
