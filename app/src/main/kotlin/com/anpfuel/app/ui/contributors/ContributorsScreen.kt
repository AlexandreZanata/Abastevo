package com.anpfuel.app.ui.contributors

import android.content.ActivityNotFoundException
import android.content.Intent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.components.SkeletonLine
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.contributor.Contributor
import com.anpfuel.domain.contributor.ContributorLinkHosts

/**
 * B-BR-CTB public contributors list (CTB-09/10).
 *
 * Anonymous read: name, verified-GitHub badge, role labels and up to two
 * link chips per contributor. Links open only the server `url` after
 * re-checking scheme `https` and the CTB-08 host allowlist, through an
 * `ACTION_VIEW` intent; no WebView is used.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContributorsScreen(
    onNavigateBack: () -> Unit,
    onClaimClick: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ContributorsViewModel = hiltViewModel(),
    onOpenLink: (String) -> Unit = defaultOpenLink(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    LaunchedEffect(viewModel) { viewModel.refresh() }
    ContributorsContent(
        state = state,
        onNavigateBack = onNavigateBack,
        onClaimClick = onClaimClick,
        onRetry = viewModel::refresh,
        onLoadMore = viewModel::loadMore,
        onOpenLink = onOpenLink,
        modifier = modifier,
    )
}

@Composable
private fun defaultOpenLink(): (String) -> Unit {
    val context = LocalContext.current
    return { url ->
        if (ContributorLinkHosts.isAllowedUrl(url)) {
            try {
                context.startActivity(Intent(Intent.ACTION_VIEW, url.toUri()))
            } catch (_: ActivityNotFoundException) {
                // No browser installed: the chip stays inert instead of crashing.
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContributorsContent(
    state: ContributorsUiState,
    onNavigateBack: () -> Unit,
    onClaimClick: () -> Unit,
    onRetry: () -> Unit,
    onLoadMore: () -> Unit,
    onOpenLink: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.contributors_screen_title)) },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { padding ->
        when {
            state.loading -> SkeletonGroup(modifier = Modifier.fillMaxWidth().padding(padding)) {
                SkeletonCard(height = 96.dp)
                SkeletonCard(height = 96.dp)
                SkeletonLine(width = 180.dp, height = 20.dp)
            }

            state.notice != null -> ContributorsNotice(
                notice = state.notice,
                onRetry = onRetry,
                modifier = Modifier.fillMaxSize().padding(padding),
            )

            state.items.isEmpty() -> ContributorsEmpty(
                onClaimClick = onClaimClick,
                modifier = Modifier.fillMaxSize().padding(padding),
            )

            else -> LazyColumn(
                modifier = Modifier.fillMaxSize().padding(padding),
                contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                item(key = "claim-entry") {
                    OutlinedButton(onClick = onClaimClick, modifier = Modifier.fillMaxWidth()) {
                        Text(stringResource(R.string.claim_entry_button))
                    }
                }
                // The public API exposes no id and names are not unique; the
                // position is stable because pages only append.
                itemsIndexed(state.items, key = { index, _ -> "contributor:$index" }) { _, item ->
                    ContributorRow(item = item, onOpenLink = onOpenLink)
                }
                if (state.hasMore) {
                    item(key = "more") {
                        OutlinedButton(
                            onClick = onLoadMore,
                            enabled = !state.loadingMore,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(stringResource(R.string.contributors_load_more))
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ContributorsNotice(
    notice: ContributorsNotice,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier.padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = stringResource(
                when (notice) {
                    ContributorsNotice.OFFLINE -> R.string.contributors_offline
                    ContributorsNotice.DENIED -> R.string.contributors_denied
                    ContributorsNotice.ERROR -> R.string.contributors_error
                },
            ),
            style = MaterialTheme.typography.bodyLarge,
        )
        Button(onClick = onRetry) {
            Text(stringResource(R.string.contributors_retry))
        }
    }
}

@Composable
private fun ContributorsEmpty(onClaimClick: () -> Unit, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier.padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = stringResource(R.string.contributors_empty_title),
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.semantics { heading() },
        )
        Text(
            text = stringResource(R.string.contributors_empty_message),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        // The first contributor must be able to claim while the list is empty.
        OutlinedButton(
            onClick = onClaimClick,
            modifier = Modifier.fillMaxWidth().testTag(CONTRIBUTORS_CLAIM_ENTRY_TAG),
        ) {
            Text(stringResource(R.string.claim_entry_button))
        }
    }
}

const val CONTRIBUTORS_CLAIM_ENTRY_TAG = "contributors-claim-entry"

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ContributorRow(
    item: Contributor,
    onOpenLink: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val containerColor = MaterialTheme.colorScheme.surface
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
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    text = item.displayName,
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f),
                )
                if (item.githubLogin != null) {
                    Icon(
                        imageVector = Icons.Default.Check,
                        contentDescription = stringResource(R.string.contributors_verified_badge),
                        tint = MaterialTheme.colorScheme.primary,
                    )
                }
            }
            if (item.contributorSince.isNotEmpty()) {
                Text(
                    text = stringResource(R.string.contributors_since, item.contributorSince),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (item.roles.isNotEmpty()) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    for (role in item.roles) {
                        AssistChip(onClick = {}, label = { Text(role) })
                    }
                }
            }
            if (item.links.isNotEmpty()) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    for (link in item.links.take(2)) {
                        AssistChip(
                            onClick = { onOpenLink(link.url) },
                            label = { Text(link.handle) },
                        )
                    }
                }
            }
        }
    }
}
