package com.anpfuel.app.ui.stations

import android.widget.Toast
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Business
import androidx.compose.material.icons.filled.LocationOn
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.community.CommunityPriceDisplay
import com.anpfuel.app.mapper.FuelProductI18n
import com.anpfuel.app.navigation.MapAppChooser
import com.anpfuel.app.navigation.MapNavigationResult
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.FuelProductIcon
import com.anpfuel.app.ui.components.PriceSourceKind
import com.anpfuel.app.ui.components.SourceTimeBadge
import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.profile.ProfileBadge
import com.anpfuel.domain.profile.ProfileBadgeInput
import com.anpfuel.domain.profile.ProfileBadgeRule
import com.anpfuel.domain.profile.StationArtwork
import com.anpfuel.domain.valueobject.FuelProduct

@Composable
fun StationPageScreen(
    onBack: () -> Unit,
    onSignIn: () -> Unit,
    onUpdatePrice: (String, String) -> Unit,
    viewModel: StationPageViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val locale = LocalConfiguration.current.locales[0]
    val context = LocalContext.current
    LaunchedEffect(viewModel, locale) { viewModel.load(locale) }
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { viewModel.refreshAccount() }
    StationPageContent(state, onBack, { viewModel.load(locale) }, { viewModel.selectFuel(it, locale) },
        onSignIn, onUpdatePrice, onRoute = {
            val station = state.canonical
            val query = state.navigationQuery ?: station?.takeIf {
                it.locationQuality == StationLocationQuality.REVIEWED && it.latitude != null && it.longitude != null
            }?.let { "${it.latitude},${it.longitude}" }
            if (query != null && MapAppChooser.openNavigation(context, query) == MapNavigationResult.NoAppFound) {
                Toast.makeText(context, context.getString(R.string.stations_navigate_no_app), Toast.LENGTH_SHORT).show()
            }
        })
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
internal fun StationPageContent(
    state: StationPageState,
    onBack: () -> Unit,
    onRetry: () -> Unit,
    onFuel: (FuelProduct) -> Unit,
    onSignIn: () -> Unit,
    onUpdatePrice: (String, String) -> Unit,
    onRoute: () -> Unit,
) {
    var showGuide by remember { mutableStateOf(false) }
    val artwork = state.profile?.artwork ?: state.canonical?.artwork ?: StationArtwork.BETA_DEFAULT
    AnpScaffold(topBar = {
        AnpTopAppBar(onNavigateUp = onBack, title = {
            Text(stringResource(R.string.station_profile_title), maxLines = 1, overflow = TextOverflow.Ellipsis)
        })
    }) { padding ->
        LazyColumn(Modifier.fillMaxSize().padding(padding).imePadding(),
            contentPadding = PaddingValues(horizontal = 20.dp, vertical = 20.dp),
            verticalArrangement = Arrangement.spacedBy(20.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            item { StationArtworkHeader(artwork, Modifier.widthIn(max = 720.dp).fillMaxWidth()) }
            item {
                Column(Modifier.widthIn(max = 720.dp).fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    if (state.loading) LinearProgressIndicator(Modifier.fillMaxWidth())
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(14.dp)) {
                        StationArtworkIcon(Modifier.size(68.dp), artwork)
                        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Text(state.name.ifEmpty { stringResource(R.string.station_profile_title) },
                                style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold,
                                modifier = Modifier.semantics { heading() })
                            state.brand?.takeIf { it.isNotBlank() }?.let {
                                Text(stringResource(R.string.stations_brand_label, it), style = MaterialTheme.typography.labelLarge,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                            val badge = state.profile?.let { ProfileBadgeRule.resolve(ProfileBadgeInput(
                                it.stationId, if (it.hasBadge) "verified" else "unclaimed", it.operatorSource,
                                it.hasBadge, state.identityStale)) }
                            if (badge is ProfileBadge.Verified) {
                                Text(stringResource(R.string.station_profile_verified), style = MaterialTheme.typography.labelLarge)
                            }
                        }
                    }
                    if (state.address.isNotEmpty() || state.location.isNotEmpty()) {
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Icon(Icons.Default.LocationOn, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
                            Column {
                                if (state.address.isNotEmpty()) Text(state.address, style = MaterialTheme.typography.bodyMedium)
                                if (state.location.isNotEmpty()) Text(state.location, style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                    }
                    if (state.identityStale) Text(stringResource(R.string.server_station_cached), color = MaterialTheme.colorScheme.error)
                    if (state.unavailable) {
                        Text(stringResource(R.string.station_profile_unavailable))
                        OutlinedButton(onClick = onRetry, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.action_retry)) }
                    }
                    val hasRoute = state.navigationQuery != null || state.canonical?.let {
                        it.locationQuality == StationLocationQuality.REVIEWED && it.latitude != null && it.longitude != null
                    } == true
                    Button(onClick = onRoute, enabled = hasRoute, modifier = Modifier.fillMaxWidth()) {
                        Text(stringResource(R.string.stations_navigate_action))
                    }
                    OutlinedButton(onClick = { state.canonical?.let { onUpdatePrice(it.stationId, WireFuelMapper.toWire(state.fuel)) } },
                        enabled = state.canParticipate, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.nav_update_price)) }
                }
            }
            item {
                FlowRow(Modifier.widthIn(max = 720.dp).fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    FuelProduct.entries.forEach { fuel ->
                        FilterChip(selected = fuel == state.fuel, onClick = { onFuel(fuel) },
                            leadingIcon = { FuelProductIcon(fuel, size = 18.dp, contentDescription = null) },
                            label = { Text(stringResource(FuelProductI18n.toStringRes(fuel))) })
                    }
                }
            }
            item {
                StationSectionCard(stringResource(R.string.community_section_title)) {
                    Text(stringResource(when {
                        state.loading -> R.string.station_page_feedback_loading
                        state.priceUnavailable -> R.string.community_unavailable_retry
                        else -> R.string.station_page_community_empty
                    }))
                    if (state.priceUnavailable && !state.loading) TextButton(onClick = onRetry) {
                        Text(stringResource(R.string.action_retry))
                    }
                    Text(stringResource(R.string.community_confidence_note), style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                    if (state.pricesStale) Text(stringResource(R.string.community_stale_label))
                }
            }
            item {
                StationSectionCard(stringResource(R.string.station_detail_anp_title)) {
                    val local = state.localDetail
                    if (local != null) {
                        Text(local.station.priceFormatted, style = MaterialTheme.typography.headlineLarge,
                            color = MaterialTheme.colorScheme.primary, fontWeight = FontWeight.Bold)
                        Text(when (state.fuel) {
                            FuelProduct.CNG -> "R$ / m³"
                            FuelProduct.LPG_P13 -> "R$ / 13 kg"
                            else -> "R$ / L"
                        }, style = MaterialTheme.typography.bodySmall)
                        local.station.collectedAtLabel?.let { Text(stringResource(R.string.stations_collected_at_label, it)) }
                        if (local.dateUnknown) Text(stringResource(R.string.station_detail_unknown_date))
                        local.surveyWeekLabel?.let { SourceTimeBadge(PriceSourceKind.ANP_DATED, it) }
                        if (local.isStale) Text(stringResource(R.string.station_detail_stale_badge), color = MaterialTheme.colorScheme.error)
                    } else {
                        val groups = state.officialGroups.filter { it.official != null }
                        if (groups.isEmpty()) Text(stringResource(if (state.loading) R.string.station_page_feedback_loading else R.string.community_official_empty))
                        groups.forEach { group ->
                            val official = checkNotNull(group.official)
                            Text(CommunityPriceDisplay.formatMilliBrl(official.amountMilliBrl) + " / " + group.unit,
                                style = MaterialTheme.typography.headlineMedium, color = MaterialTheme.colorScheme.primary)
                            Text(stringResource(R.string.stations_collected_at_label, official.collectedOn))
                            SourceTimeBadge(PriceSourceKind.ANP_DATED, "${official.surveyWeekStart} – ${official.surveyWeekEnd}")
                            if (group.conditionKind != "STANDARD") Text(
                                CommunityPriceDisplay.formatCondition(group.conditionKind, null),
                                style = MaterialTheme.typography.labelSmall)
                            if (group.conditionKind == "STANDARD") Text(stringResource(when (state.fuel) {
                                FuelProduct.CNG -> R.string.feed_unit_m3
                                FuelProduct.LPG_P13 -> R.string.feed_unit_cylinder
                                else -> R.string.feed_unit_litre
                            }), style = MaterialTheme.typography.labelSmall)
                        }
                    }
                    if (state.pricesStale) Text(stringResource(R.string.community_stale_label))
                }
            }
            item {
                if (state.canParticipate) {
                    val id = checkNotNull(state.canonical).stationId
                    key(id, state.fuel, state.accountId) {
                        StationExperienceSection(id, WireFuelMapper.toWire(state.fuel), state.accountId, onSignIn,
                            Modifier.widthIn(max = 720.dp).fillMaxWidth())
                    }
                } else {
                    StationSectionCard(stringResource(R.string.station_page_experience_title)) {
                        Text(stringResource(R.string.station_page_participation_pending))
                        OutlinedButton(onClick = onRetry, enabled = !state.loading) { Text(stringResource(R.string.action_retry)) }
                    }
                }
            }
            state.profile?.business?.takeIf { it.isNotEmpty() }?.let { business ->
                item {
                    StationSectionCard(stringResource(R.string.station_profile_title)) {
                        business.forEach { (field, value) ->
                            val label = when (field) {
                                "opening_hours" -> R.string.station_profile_hours
                                "services" -> R.string.station_profile_services
                                "phone" -> R.string.station_profile_phone
                                "website" -> R.string.station_profile_website
                                else -> R.string.station_profile_description
                            }
                            Text(stringResource(label), style = MaterialTheme.typography.labelLarge)
                            Text(value)
                        }
                    }
                }
            }
            item {
                Card(Modifier.widthIn(max = 720.dp).fillMaxWidth(), shape = RoundedCornerShape(24.dp),
                    colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.secondaryContainer)) {
                    Column(Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        Icon(Icons.Default.Business, contentDescription = null)
                        Text(stringResource(R.string.station_page_owner_title), style = MaterialTheme.typography.titleLarge,
                            fontWeight = FontWeight.Bold, modifier = Modifier.semantics { heading() })
                        Text(stringResource(R.string.station_page_owner_copy))
                        OutlinedButton(onClick = { showGuide = true }, modifier = Modifier.fillMaxWidth()) {
                            Text(stringResource(R.string.station_page_owner_action))
                        }
                    }
                }
            }
        }
    }
    if (showGuide) {
        AlertDialog(onDismissRequest = { showGuide = false }, title = { Text(stringResource(R.string.station_page_guide_title)) },
            text = {
                Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                    for (label in listOf(R.string.station_page_guide_intro, R.string.station_page_guide_account,
                        R.string.station_page_guide_authority, R.string.station_page_guide_review, R.string.station_page_guide_beta)) {
                        Text(stringResource(label))
                    }
                    Text(stringResource(R.string.station_profile_representation_note), style = MaterialTheme.typography.bodySmall)
                }
            }, confirmButton = { TextButton(onClick = { showGuide = false }) { Text(stringResource(R.string.action_back)) } })
    }
}

@Composable
internal fun StationSectionCard(title: String, modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Card(modifier.widthIn(max = 720.dp).fillMaxWidth(), shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow)) {
        Column(Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(title, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold,
                modifier = Modifier.semantics { heading() })
            content()
        }
    }
}
