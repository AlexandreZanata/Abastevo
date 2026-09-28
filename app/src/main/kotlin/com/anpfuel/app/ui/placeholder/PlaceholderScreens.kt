package com.anpfuel.app.ui.placeholder

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import com.anpfuel.app.ui.components.AnpScaffold
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpAttributionFooter

@Composable
fun PlaceholderScreen(
    titleRes: Int,
    modifier: Modifier = Modifier,
    showAttribution: Boolean = false,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        bottomBar = {
            if (showAttribution) {
                AnpAttributionFooter()
            }
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .padding(24.dp),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                text = stringResource(titleRes),
                style = MaterialTheme.typography.headlineMedium,
                color = MaterialTheme.colorScheme.onBackground,
            )
            Text(
                text = stringResource(R.string.screen_coming_soon),
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 12.dp),
            )
        }
    }
}

@Composable
fun OnboardingPlaceholderScreen(modifier: Modifier = Modifier) {
    PlaceholderScreen(
        titleRes = R.string.nav_onboarding,
        modifier = modifier,
    )
}

@Composable
fun SearchPlaceholderScreen(modifier: Modifier = Modifier) {
    PlaceholderScreen(
        titleRes = R.string.nav_search,
        modifier = modifier,
    )
}

@Composable
fun LocationPlaceholderScreen(modifier: Modifier = Modifier) {
    PlaceholderScreen(
        titleRes = R.string.nav_location,
        modifier = modifier,
    )
}

@Composable
fun PricesPlaceholderScreen(modifier: Modifier = Modifier) {
    PlaceholderScreen(
        titleRes = R.string.nav_prices,
        modifier = modifier,
        showAttribution = true,
    )
}

@Composable
fun HistoryPlaceholderScreen(modifier: Modifier = Modifier) {
    PlaceholderScreen(
        titleRes = R.string.nav_history,
        modifier = modifier,
        showAttribution = true,
    )
}

@Composable
fun StationsPlaceholderScreen(modifier: Modifier = Modifier) {
    PlaceholderScreen(
        titleRes = R.string.nav_stations,
        modifier = modifier,
        showAttribution = true,
    )
}

@Composable
fun SettingsPlaceholderScreen(modifier: Modifier = Modifier) {
    PlaceholderScreen(
        titleRes = R.string.nav_settings,
        modifier = modifier,
    )
}
