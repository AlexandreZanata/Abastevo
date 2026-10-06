package com.anpfuel.app.ui.community

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material.icons.automirrored.filled.TrendingDown
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.navigation.Routes
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.FuelProductIcon
import com.anpfuel.app.ui.theme.FuelProductTint
import com.anpfuel.domain.community.CommunityFeedItem
import com.anpfuel.domain.community.FeedSort
import com.anpfuel.domain.valueobject.FuelProduct
import java.math.BigDecimal
import java.text.NumberFormat
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

@Composable
fun CommunityScreen(darkTheme: Boolean, onToggleTheme: () -> Unit, onNavigate: (String) -> Unit,
    modifier: Modifier = Modifier, viewModel: CommunityFeedViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    DisposableEffect(lifecycle, viewModel) {
        val observer = LifecycleEventObserver { _, _ -> viewModel.setForeground(lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) }
        lifecycle.addObserver(observer)
        viewModel.setForeground(lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED))
        onDispose { lifecycle.removeObserver(observer); viewModel.setForeground(false) }
    }
    CommunityFeedContent(state, darkTheme, onToggleTheme, onNavigate, viewModel::selectFuel,
        viewModel::selectSort, viewModel::refresh, viewModel::loadMore, modifier)
}

/** Home's native surfaces/icons with bounded, source-labelled price activity. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun CommunityFeedContent(state: CommunityFeedUiState, darkTheme: Boolean, onToggleTheme: () -> Unit,
    onNavigate: (String) -> Unit, onFuel: (FuelProduct) -> Unit, onSort: (FeedSort) -> Unit,
    onRefresh: () -> Unit, onMore: () -> Unit, modifier: Modifier = Modifier) {
    var showRules by remember { mutableStateOf(false) }
    AnpScaffold(modifier = modifier.fillMaxSize(), topBar = {
        AnpTopAppBar(title = {
            Text(stringResource(R.string.community_screen_title))
        }, actions = {
            IconButton(onClick = onRefresh, enabled = state.city != null && !state.loading) {
                Icon(Icons.Default.Refresh, stringResource(R.string.feed_refresh))
            }
            IconButton(onClick = onToggleTheme) {
                Icon(if (darkTheme) Icons.Default.LightMode else Icons.Default.DarkMode,
                    stringResource(if (darkTheme) R.string.action_switch_light_theme else R.string.action_switch_dark_theme))
            }
        })
    }) { padding ->
        LazyColumn(Modifier.fillMaxSize().padding(padding), contentPadding = PaddingValues(16.dp, 12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)) {
            item {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Column(Modifier.weight(1f)) {
                        Text(stringResource(R.string.feed_your_city), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        Text(state.city?.let { "${it.name}, ${it.state.abbreviation}" } ?: stringResource(R.string.feed_choose_city),
                            style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.semantics { heading() })
                    }
                    TextButton(onClick = { onNavigate(Routes.LOCATION) }) { Text(stringResource(R.string.feed_change_city)) }
                }
            }
            item {
                Card(shape = RoundedCornerShape(24.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.primaryContainer)) {
                    Column(Modifier.fillMaxWidth().padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                            Surface(shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.surface, modifier = Modifier.size(48.dp)) {
                                Box(contentAlignment = Alignment.Center) { Icon(painterResource(R.drawable.ic_home_community), null, tint = Color.Unspecified, modifier = Modifier.size(30.dp)) }
                            }
                            Column(Modifier.weight(1f)) {
                                Text(stringResource(R.string.feed_contribute_title), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
                                Text(stringResource(R.string.feed_contribute_hint), style = MaterialTheme.typography.bodySmall)
                            }
                        }
                        Button(onClick = { onNavigate(Routes.CAPTURE) }, modifier = Modifier.fillMaxWidth()) {
                            Icon(Icons.Default.Add, null, Modifier.size(20.dp)); Spacer(Modifier.width(8.dp))
                            Text(stringResource(R.string.community_action_contribute))
                        }
                    }
                }
            }
            item {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(stringResource(R.string.feed_prices_title), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, modifier = Modifier.semantics { heading() })
                    Text(stringResource(R.string.feed_auto_refresh), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            item {
                Row(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                ) {
                    FilterChip(selected = state.sort == FeedSort.RECENT, onClick = { onSort(FeedSort.RECENT) },
                        label = { Text(stringResource(R.string.feed_recent)) }, leadingIcon = { Icon(Icons.Default.Schedule, null, Modifier.size(18.dp)) })
                    FilterChip(selected = state.sort == FeedSort.CHEAPEST, onClick = { onSort(FeedSort.CHEAPEST) },
                        label = { Text(stringResource(R.string.feed_cheapest)) }, leadingIcon = { Icon(Icons.AutoMirrored.Filled.TrendingDown, null, Modifier.size(18.dp)) })
                    FilterChip(selected = state.sort == FeedSort.BEST, onClick = { onSort(FeedSort.BEST) },
                        label = { Text(stringResource(R.string.feed_best)) }, leadingIcon = { Icon(Icons.Default.Star, null, Modifier.size(18.dp)) })
                    FilterChip(selected = state.sort == FeedSort.WORST, onClick = { onSort(FeedSort.WORST) },
                        label = { Text(stringResource(R.string.feed_worst)) }, leadingIcon = { Icon(Icons.Default.StarOutline, null, Modifier.size(18.dp)) })
                }
            }
            item {
                LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(FuelProduct.entries, key = { it.name }) { fuel ->
                        FilterChip(selected = state.fuel == fuel, onClick = { onFuel(fuel) },
                            label = { Text(stringResource(FuelProductI18n.toStringRes(fuel))) },
                            leadingIcon = { FuelProductIcon(fuel, size = 20.dp, contentDescription = null) })
                    }
                }
            }
            if (state.items.size > 20 && !state.failed) item {
                Text(stringResource(R.string.feed_cached_hint), style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            if (state.newUpdates) item {
                OutlinedButton(onClick = onRefresh, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.feed_new_updates)) }
            }
            if (state.failed) item {
                FeedMessage(stringResource(R.string.feed_unavailable),
                    stringResource(if (state.items.isEmpty()) R.string.feed_unavailable_hint else R.string.feed_cached_hint)) {
                    TextButton(onClick = onRefresh) { Text(stringResource(R.string.feed_retry)) }
                }
            }
            when {
                state.loading -> item { Box(Modifier.fillMaxWidth().padding(24.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator() } }
                state.noCity -> item { FeedMessage(stringResource(R.string.feed_choose_city), stringResource(R.string.feed_no_city_hint)) {
                    TextButton(onClick = { onNavigate(Routes.LOCATION) }) { Text(stringResource(R.string.feed_choose_city)) }
                } }
                state.items.isEmpty() && !state.failed -> item {
                    FeedMessage(stringResource(R.string.feed_empty_title), stringResource(R.string.feed_empty_hint)) {}
                }
            }
            items(state.items, key = { it.stationId }) { item ->
                FeedPriceCard(item, darkTheme, onOpen = { onNavigate(Routes.stationPage(item.stationId, state.fuel)) })
            }
            if (state.nextCursor != null && state.items.size < 200 && !state.loading) item {
                OutlinedButton(onClick = onMore, enabled = !state.loadingMore, modifier = Modifier.fillMaxWidth()) {
                    if (state.loadingMore) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                    else Text(stringResource(R.string.feed_load_more))
                }
            }
            item {
                TextButton(onClick = { showRules = true }, modifier = Modifier.fillMaxWidth()) {
                    Icon(Icons.Default.Info, null, Modifier.size(18.dp)); Spacer(Modifier.width(8.dp))
                    Text(stringResource(R.string.community_rules_title))
                }
            }
        }
    }
    if (showRules) AlertDialog(onDismissRequest = { showRules = false }, title = { Text(stringResource(R.string.community_rules_title)) },
        text = { Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.community_rules_body)); Text(stringResource(R.string.community_moderation_body)); Text(stringResource(R.string.community_moderation_appeal))
        } }, confirmButton = { TextButton(onClick = { showRules = false }) { Text(stringResource(R.string.feed_close)) } })
}

@Composable
private fun FeedMessage(title: String, description: String, action: @Composable () -> Unit) {
    Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(20.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
            Text(description, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            action()
        }
    }
}

@Composable
private fun FeedPriceCard(item: CommunityFeedItem, darkTheme: Boolean, onOpen: () -> Unit) {
    val locale = LocalConfiguration.current.locales[0]
    val timestamp = DateTimeFormatter.ofPattern("d MMM · HH:mm", locale).withZone(ZoneId.systemDefault()).format(item.updatedAt)
    val price = remember(item.amountMilliBrl) {
        NumberFormat.getCurrencyInstance(Locale.forLanguageTag("pt-BR")).apply {
            minimumFractionDigits = 2; maximumFractionDigits = 3
        }.format(BigDecimal.valueOf(item.amountMilliBrl, 3))
    }
    Card(onClick = onOpen, modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Icon(painterResource(R.drawable.ic_home_community), null, tint = Color.Unspecified, modifier = Modifier.size(24.dp))
                Column(Modifier.weight(1f)) {
                    Text(item.stationName, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
                    Text("${stringResource(R.string.feed_community_source)} · $timestamp", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Icon(Icons.Default.ChevronRight, stringResource(R.string.feed_station_detail), Modifier.size(20.dp))
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.5f))
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                FuelProductIcon(item.fuel, size = 44.dp, contentDescription = null)
                Column(Modifier.weight(1f)) {
                    Text(stringResource(FuelProductI18n.toStringRes(item.fuel)), style = MaterialTheme.typography.bodyMedium)
                    Text(price, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold, color = FuelProductTint.colorFor(item.fuel, darkTheme))
                    Text(stringResource(when (item.unit) { "M3" -> R.string.feed_unit_m3; "KG_13" -> R.string.feed_unit_cylinder; else -> R.string.feed_unit_litre }),
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            val avg = item.ratingsAvg
            if (avg != null) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    Icon(Icons.Default.Star, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(16.dp))
                    Text(stringResource(R.string.feed_rating_value, avg, item.ratingsCount),
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            Text(stringResource(when (item.confidence) { "HIGH" -> R.string.feed_confidence_high; "MEDIUM" -> R.string.feed_confidence_medium; else -> R.string.feed_confidence_low }),
                style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
