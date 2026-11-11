package com.anpfuel.app.ui.forum

import com.anpfuel.app.ui.components.skeletonShimmer
import com.anpfuel.app.ui.components.SkeletonLine
import com.anpfuel.app.ui.components.EmptyState
import androidx.compose.material3.TabRowDefaults.tabIndicatorOffset
import androidx.compose.material3.TabRowDefaults
import androidx.compose.material3.Tab
import androidx.compose.material3.ScrollableTabRow
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.Badge
import androidx.compose.material.icons.outlined.Share
import androidx.compose.material.icons.outlined.Forum
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material.icons.outlined.ChatBubbleOutline
import androidx.compose.material.icons.filled.Tune
import androidx.compose.material.icons.filled.Link
import androidx.compose.material.icons.filled.Edit
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.clickable
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
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
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
    val share = rememberForumShare()
    val categoryNames = state.categories.associate { it.rawCode to it.name }
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.surface,
        topBar = {
            Column {
                AnpTopAppBar(
                    title = { Text(stringResource(R.string.forum_screen_title)) },
                    onNavigateUp = onBack,
                    actions = {
                        IconButton(onClick = { showFilterDialog = true }) {
                            BadgedBox(
                                badge = {
                                    if (state.hasActiveFilters) {
                                        Badge(containerColor = MaterialTheme.colorScheme.secondary)
                                    }
                                },
                            ) {
                                Icon(Icons.Default.Tune, contentDescription = stringResource(R.string.forum_filters))
                            }
                        }
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
                ForumCategoryTabs(
                    state = state,
                    onSelectCategory = onSelectCategory,
                )
            }
        },
        floatingActionButton = {
            // Round compose button, like the post button of social feeds.
            FloatingActionButton(
                onClick = onNewTopic,
                shape = CircleShape,
                containerColor = MaterialTheme.colorScheme.primary,
                contentColor = MaterialTheme.colorScheme.onPrimary,
            ) {
                Icon(
                    imageVector = Icons.Default.Edit,
                    contentDescription = stringResource(R.string.forum_new_topic),
                )
            }
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
            contentPadding = PaddingValues(bottom = 96.dp),
        ) {
            if (state.hasActiveFilters) {
                item {
                    Row(
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 16.dp, vertical = 8.dp),
                    ) {
                        FilterChip(
                            selected = true,
                            onClick = { showFilterDialog = true },
                            label = { Text(stringResource(R.string.forum_filters)) },
                            leadingIcon = {
                                Icon(Icons.Default.Tune, contentDescription = null, modifier = Modifier.size(18.dp))
                            },
                        )
                        TextButton(onClick = onClearFilters) {
                            Text(stringResource(R.string.forum_filter_clear))
                        }
                    }
                    ForumDivider()
                }
            }

            if (state.isOffline) {
                item {
                    Text(
                        text = stringResource(R.string.forum_offline_notice),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
                    )
                    ForumDivider()
                }
            }

            when {
                state.loading && state.items.isEmpty() -> {
                    item { ForumFeedSkeleton() }
                }
                state.error != null && state.items.isEmpty() -> {
                    item {
                        EmptyState(
                            message = stringResource(R.string.forum_unavailable_title),
                            hint = stringResource(R.string.forum_unavailable_hint),
                            icon = Icons.Outlined.CloudOff,
                            actionLabel = stringResource(R.string.forum_retry),
                            onAction = onRefresh,
                            modifier = Modifier.padding(16.dp),
                        )
                    }
                }
                !state.loading && state.items.isEmpty() -> {
                    item {
                        EmptyState(
                            message = stringResource(R.string.forum_feed_empty_title),
                            hint = stringResource(
                                if (state.hasActiveFilters) {
                                    R.string.forum_feed_empty_filtered_hint
                                } else {
                                    R.string.forum_feed_empty_hint
                                },
                            ),
                            icon = Icons.Outlined.Forum,
                            actionLabel = stringResource(R.string.forum_new_topic),
                            onAction = onNewTopic,
                            modifier = Modifier.padding(16.dp),
                        )
                    }
                }
            }

            items(state.items, key = { it.id }) { post ->
                ForumPostCard(
                    post = post,
                    categoryName = categoryNames[post.rawCategory],
                    onClick = { onOpenPost(post.id) },
                    onShare = { share(post.title, post.excerpt) },
                )
                ForumDivider()
            }

            if (state.nextCursor != null && !state.loading) {
                item {
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(16.dp),
                        contentAlignment = Alignment.Center,
                    ) {
                        if (state.loadingMore) {
                            CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 2.dp)
                        } else {
                            OutlinedButton(onClick = onLoadMore) {
                                Text(stringResource(R.string.forum_load_more))
                            }
                        }
                    }
                }
            }
        }
    }
}

/** "Todos" plus each category as underline tabs under the top bar. */
@Composable
private fun ForumCategoryTabs(
    state: ForumFeedUiState,
    onSelectCategory: (ForumCategory?) -> Unit,
) {
    val scheme = MaterialTheme.colorScheme
    val tabs = listOf<Pair<ForumCategory?, String>>(null to stringResource(R.string.forum_category_all)) +
        state.categories.map { it.code to it.name }
    val selectedIndex = tabs.indexOfFirst { it.first == state.selectedCategory }.coerceAtLeast(0)
    Column {
        ScrollableTabRow(
            selectedTabIndex = selectedIndex,
            edgePadding = 8.dp,
            containerColor = scheme.surface,
            contentColor = scheme.onSurface,
            indicator = { positions ->
                if (selectedIndex < positions.size) {
                    TabRowDefaults.PrimaryIndicator(
                        modifier = Modifier.tabIndicatorOffset(positions[selectedIndex]),
                        width = 48.dp,
                        height = 4.dp,
                        color = scheme.onSurface,
                        shape = RoundedCornerShape(topStart = 4.dp, topEnd = 4.dp),
                    )
                }
            },
            divider = {},
        ) {
            tabs.forEachIndexed { index, (category, name) ->
                val selected = index == selectedIndex
                Tab(
                    selected = selected,
                    onClick = { onSelectCategory(category) },
                    selectedContentColor = scheme.onSurface,
                    unselectedContentColor = scheme.onSurfaceVariant,
                    text = {
                        Text(
                            text = name,
                            style = MaterialTheme.typography.titleSmall,
                            fontWeight = if (selected) FontWeight.Bold else FontWeight.Medium,
                            maxLines = 1,
                        )
                    },
                )
            }
        }
        ForumDivider()
    }
}

@Composable
internal fun ForumDivider(modifier: Modifier = Modifier) {
    HorizontalDivider(modifier = modifier, color = MaterialTheme.colorScheme.outlineVariant)
}

@Composable
private fun ForumFeedSkeleton() {
    SkeletonGroup(modifier = Modifier.fillMaxWidth().padding(16.dp)) {
        repeat(4) {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Box(Modifier.size(44.dp).then(skeletonShimmer(cornerRadius = 22.dp)))
                Column(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    SkeletonLine(width = 160.dp, height = 14.dp)
                    SkeletonLine(height = 18.dp)
                    SkeletonLine(height = 14.dp)
                    SkeletonLine(width = 200.dp, height = 14.dp)
                }
            }
        }
    }
}

/** One post of the feed: avatar, author line, kind and category, title, excerpt and actions. */
@Composable
internal fun ForumPostCard(
    post: ForumPostSummary,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    categoryName: String? = null,
    onShare: () -> Unit = {},
) {
    val scheme = MaterialTheme.colorScheme
    val author = forumPostAuthor(post.kind, post.publicAlias)
    Row(
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(start = 16.dp, end = 8.dp, top = 12.dp, bottom = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        ForumAvatar(author)
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            ForumAuthorLine(author = author, timeIso = post.firstPublishedAt, modifier = Modifier.padding(end = 8.dp))
            Text(
                text = listOfNotNull(stringResource(forumKindLabel(post.kind)), categoryName).joinToString(" · "),
                style = MaterialTheme.typography.labelMedium,
                color = scheme.onSurfaceVariant,
            )
            Text(
                text = post.title,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Bold,
                color = scheme.onSurface,
                modifier = Modifier.padding(top = 2.dp, end = 8.dp),
            )
            if (post.excerpt.isNotBlank()) {
                Text(
                    text = post.excerpt,
                    style = MaterialTheme.typography.bodyMedium,
                    color = scheme.onSurface,
                    maxLines = 4,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(end = 8.dp),
                )
            }
            post.source?.let { source ->
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    Icon(
                        Icons.Default.Link,
                        contentDescription = null,
                        tint = scheme.onSurfaceVariant,
                        modifier = Modifier.size(16.dp),
                    )
                    Text(
                        text = source.name,
                        style = MaterialTheme.typography.labelMedium,
                        color = scheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (post.commentsEnabled) {
                    ForumIconAction(
                        icon = Icons.Outlined.ChatBubbleOutline,
                        contentDescription = stringResource(R.string.forum_action_comment),
                        onClick = onClick,
                    )
                } else {
                    Spacer(Modifier.size(40.dp))
                }
                ForumIconAction(
                    icon = Icons.Outlined.Share,
                    contentDescription = stringResource(R.string.forum_action_share),
                    onClick = onShare,
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
