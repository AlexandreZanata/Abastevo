package com.anpfuel.app.ui.forum

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.SuggestionChipDefaults
import androidx.compose.material3.Text
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.designsystem.FilterChip
import com.anpfuel.app.ui.designsystem.OutlinedButton
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.usecase.forum.SubmitForumTopicUseCase
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumFeedFilters
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumPostSummary
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

@Composable
fun ForumScreen(
    onBack: () -> Unit,
    onOpenPost: (String) -> Unit,
    modifier: Modifier = Modifier,
    onNewTopic: () -> Unit = {},
    viewModel: ForumFeedViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    ForumFeedContent(
        state = state,
        onBack = onBack,
        onOpenPost = onOpenPost,
        onSelectCategory = viewModel::selectCategory,
        onApplyFilters = viewModel::updateFilters,
        onClearFilters = viewModel::clearFilters,
        onRefresh = viewModel::refresh,
        onLoadMore = viewModel::loadMore,
        onNewTopic = onNewTopic,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun ForumFeedContent(
    state: ForumFeedUiState,
    onBack: () -> Unit,
    onOpenPost: (String) -> Unit,
    onSelectCategory: (ForumCategory?) -> Unit,
    onApplyFilters: (ForumFeedFilters) -> Unit,
    onClearFilters: () -> Unit,
    onRefresh: () -> Unit,
    onLoadMore: () -> Unit,
    modifier: Modifier = Modifier,
    onNewTopic: () -> Unit = {},
) {
    var showFilterDialog by remember { mutableStateOf(false) }
    if (showFilterDialog) {
        ForumFilterDialog(
            current = state.filters,
            availableUfs = state.availableUfs,
            availableFuels = state.availableFuels,
            onDismiss = { showFilterDialog = false },
            onApply = {
                showFilterDialog = false
                onApplyFilters(it)
            },
        )
    }
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.forum_screen_title)) },
                onNavigateUp = onBack,
                actions = {
                    IconButton(
                        onClick = onRefresh,
                        enabled = !state.loading && !state.refreshing,
                    ) {
                        Icon(
                            Icons.Default.Refresh,
                            contentDescription = stringResource(R.string.forum_retry),
                        )
                    }
                },
            )
        },
        floatingActionButton = {
            FloatingActionButton(
                onClick = onNewTopic,
                contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
                containerColor = MaterialTheme.colorScheme.primaryContainer,
            ) {
                Icon(
                    imageVector = Icons.Default.Add,
                    contentDescription = stringResource(R.string.forum_new_topic),
                )
            }
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
            contentPadding = PaddingValues(16.dp, 12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item {
                LazyRow(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    item {
                        FilterChip(
                            selected = state.selectedCategory == null,
                            onClick = { onSelectCategory(null) },
                            label = { Text(stringResource(R.string.forum_category_all)) },
                        )
                    }
                    items(state.categories, key = { it.rawCode }) { categoryItem ->
                        FilterChip(
                            selected = state.selectedCategory == categoryItem.code,
                            onClick = { onSelectCategory(categoryItem.code) },
                            label = { Text(categoryItem.name) },
                        )
                    }
                }
            }

            item {
                Row(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    FilterChip(
                        selected = state.hasActiveFilters,
                        onClick = { showFilterDialog = true },
                        label = { Text(stringResource(R.string.forum_filters)) },
                    )
                    if (state.hasActiveFilters) {
                        TextButton(onClick = onClearFilters) {
                            Text(stringResource(R.string.forum_filter_clear))
                        }
                    }
                }
            }

            if (state.isOffline) {
                item {
                    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
                    Card(
                        modifier = Modifier.fillMaxWidth(),
                        shape = AnpContainerDefaults.containerShape(),
                        colors = CardDefaults.cardColors(containerColor = containerColor),
                        border = AnpContainerDefaults.borderStroke(containerColor),
                    ) {
                        Text(
                            text = stringResource(R.string.forum_offline_notice),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(12.dp),
                        )
                    }
                }
            }

            when {
                state.loading && state.items.isEmpty() -> {
                    item {
                        SkeletonGroup(modifier = Modifier.fillMaxWidth()) {
                            repeat(3) { SkeletonCard(height = 140.dp) }
                        }
                    }
                }
                state.error != null && state.items.isEmpty() -> {
                    item {
                        val title = when (state.error) {
                            ForumUiError.DISABLED -> stringResource(R.string.forum_unavailable_title)
                            ForumUiError.OFFLINE -> stringResource(R.string.forum_unavailable_title)
                            ForumUiError.GENERIC -> stringResource(R.string.forum_unavailable_title)
                        }
                        val hint = when (state.error) {
                            ForumUiError.DISABLED -> stringResource(R.string.forum_unavailable_hint)
                            ForumUiError.OFFLINE -> stringResource(R.string.forum_unavailable_hint)
                            ForumUiError.GENERIC -> stringResource(R.string.forum_unavailable_hint)
                        }
                        ForumMessageCard(
                            title = title,
                            description = hint,
                            actionLabel = stringResource(R.string.forum_retry),
                            onAction = onRefresh,
                        )
                    }
                }
                !state.loading && state.items.isEmpty() -> {
                    item {
                        ForumMessageCard(
                            title = stringResource(R.string.forum_feed_empty_title),
                            description = stringResource(
                                if (state.hasActiveFilters) {
                                    R.string.forum_feed_empty_filtered_hint
                                } else {
                                    R.string.forum_feed_empty_hint
                                },
                            ),
                        )
                    }
                }
            }

            items(state.items, key = { it.id }) { post ->
                ForumPostCard(post = post, onClick = { onOpenPost(post.id) })
            }

            if (state.nextCursor != null && !state.loading) {
                item {
                    OutlinedButton(
                        onClick = onLoadMore,
                        enabled = !state.loadingMore,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        if (state.loadingMore) {
                            CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                        } else {
                            Text(stringResource(R.string.forum_load_more))
                        }
                    }
                }
            }
        }
    }
}

@Composable
internal fun ForumPostCard(
    post: ForumPostSummary,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        onClick = onClick,
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = containerColor),
        border = AnpContainerDefaults.borderStroke(containerColor),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                SuggestionChip(
                    onClick = {},
                    label = { Text(stringResource(forumKindLabel(post.kind))) },
                    colors = SuggestionChipDefaults.suggestionChipColors(
                        containerColor = MaterialTheme.colorScheme.secondaryContainer,
                        labelColor = MaterialTheme.colorScheme.onSecondaryContainer,
                    ),
                    border = null,
                )
                Text(
                    text = formatForumDate(post.firstPublishedAt),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            Text(
                text = post.title,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Bold,
            )

            if (post.excerpt.isNotBlank()) {
                Text(
                    text = post.excerpt,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 3,
                    overflow = TextOverflow.Ellipsis,
                )
            }

            val source = post.source
            if (source != null) {
                Text(
                    text = source.name,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
        }
    }
}

@Composable
internal fun ForumMessageCard(
    title: String,
    description: String,
    actionLabel: String? = null,
    onAction: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
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
            Text(title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
            Text(description, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (actionLabel != null && onAction != null) {
                TextButton(onClick = onAction) {
                    Text(actionLabel)
                }
            }
        }
    }
}

internal fun forumKindLabel(kind: ForumPostKind): Int = when (kind) {
    ForumPostKind.EDITORIAL -> R.string.forum_kind_editorial
    ForumPostKind.NEWS -> R.string.forum_kind_news
    ForumPostKind.COMMUNITY -> R.string.forum_card_title
    ForumPostKind.TOPIC -> R.string.forum_kind_topic
    ForumPostKind.BULLETIN -> R.string.forum_kind_bulletin
    ForumPostKind.UNKNOWN -> R.string.forum_kind_editorial
}

internal fun formatForumDate(isoString: String): String {
    return try {
        val instant = Instant.parse(isoString)
        val formatter = DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM)
            .withZone(ZoneId.systemDefault())
        formatter.format(instant)
    } catch (_: Exception) {
        isoString
    }
}

@Composable
private fun ForumFilterDialog(
    current: ForumFeedFilters,
    availableUfs: List<String>,
    availableFuels: List<String>,
    onDismiss: () -> Unit,
    onApply: (ForumFeedFilters) -> Unit,
) {
    var uf by remember(current) { mutableStateOf(current.uf) }
    var ibge by remember(current) { mutableStateOf(current.municipalityIbge?.toString().orEmpty()) }
    var fuel by remember(current) { mutableStateOf(current.fuelProduct) }
    var station by remember(current) { mutableStateOf(current.stationId.orEmpty()) }
    var validationError by remember(current) { mutableStateOf<Int?>(null) }
    var ufMenuExpanded by remember { mutableStateOf(false) }
    var fuelMenuExpanded by remember { mutableStateOf(false) }

    fun applyFilters() {
        val trimmedIbge = ibge.trim()
        val parsedIbge = if (trimmedIbge.isEmpty()) null else trimmedIbge.toIntOrNull()
        if (trimmedIbge.isNotEmpty() && (parsedIbge == null || trimmedIbge.length != 7 || parsedIbge !in 1000000..9999999)) {
            validationError = R.string.forum_filter_invalid_ibge
            return
        }
        val expectedPrefix = uf?.let { SubmitForumTopicUseCase.VALID_UFS[it] }
        if (parsedIbge != null && expectedPrefix != null && parsedIbge / 100000 != expectedPrefix) {
            validationError = R.string.forum_filter_invalid_ibge
            return
        }
        val trimmedStation = station.trim().takeIf { it.isNotEmpty() }
        if (trimmedStation != null && !SubmitForumTopicUseCase.UUID_REGEX.matches(trimmedStation)) {
            validationError = R.string.forum_filter_invalid_station
            return
        }
        validationError = null
        onApply(
            ForumFeedFilters(
                uf = uf,
                municipalityIbge = parsedIbge,
                fuelProduct = fuel,
                stationId = trimmedStation?.lowercase(),
            ),
        )
    }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.forum_filters_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Box {
                    OutlinedButton(
                        onClick = { ufMenuExpanded = true },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(uf ?: stringResource(R.string.forum_filter_all_ufs))
                    }
                    DropdownMenu(
                        expanded = ufMenuExpanded,
                        onDismissRequest = { ufMenuExpanded = false },
                    ) {
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.forum_filter_all_ufs)) },
                            onClick = {
                                uf = null
                                ufMenuExpanded = false
                            },
                        )
                        availableUfs.forEach { option ->
                            DropdownMenuItem(
                                text = { Text(option) },
                                onClick = {
                                    uf = option
                                    ufMenuExpanded = false
                                },
                            )
                        }
                    }
                }

                OutlinedTextField(
                    value = ibge,
                    onValueChange = { value ->
                        ibge = value.filter { it.isDigit() }.take(7)
                        validationError = null
                    },
                    label = { Text(stringResource(R.string.forum_filter_ibge)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                )

                Box {
                    OutlinedButton(
                        onClick = { fuelMenuExpanded = true },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(fuel ?: stringResource(R.string.forum_filter_all_fuels))
                    }
                    DropdownMenu(
                        expanded = fuelMenuExpanded,
                        onDismissRequest = { fuelMenuExpanded = false },
                    ) {
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.forum_filter_all_fuels)) },
                            onClick = {
                                fuel = null
                                fuelMenuExpanded = false
                            },
                        )
                        availableFuels.forEach { option ->
                            DropdownMenuItem(
                                text = { Text(option) },
                                onClick = {
                                    fuel = option
                                    fuelMenuExpanded = false
                                },
                            )
                        }
                    }
                }

                OutlinedTextField(
                    value = station,
                    onValueChange = { value ->
                        station = value.take(36)
                        validationError = null
                    },
                    label = { Text(stringResource(R.string.forum_filter_station)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )

                validationError?.let { errorRes ->
                    Text(
                        text = stringResource(errorRes),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { applyFilters() }) {
                Text(stringResource(R.string.forum_filter_apply))
            }
        },
        dismissButton = {
            Row {
                TextButton(onClick = { onApply(ForumFeedFilters()) }) {
                    Text(stringResource(R.string.forum_filter_clear))
                }
                TextButton(onClick = onDismiss) {
                    Text(stringResource(R.string.forum_filter_cancel))
                }
            }
        },
    )
}
