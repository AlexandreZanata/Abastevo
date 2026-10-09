package com.anpfuel.app.ui.stations

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AbastevoBrandBanner
import com.anpfuel.domain.profile.StationArtworkAsset
import com.anpfuel.domain.profile.StationArtwork

/** Shared README-style brand banner as the default station artwork; local default artwork is not a business badge. */
@Composable
internal fun StationArtworkHeader(artwork: StationArtwork, modifier: Modifier = Modifier) {
    Column(modifier, verticalArrangement = Arrangement.spacedBy(12.dp)) {
        AbastevoBrandBanner(showTagline = false)
        Text(stringResource(R.string.station_page_artwork_note), style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
internal fun StationArtworkIcon(modifier: Modifier = Modifier, artwork: StationArtwork = StationArtwork.BETA_DEFAULT) {
    val mark = when (artwork.icon) {
        StationArtworkAsset.ABASTEVO_ICON, StationArtworkAsset.ABASTEVO_BANNER -> R.drawable.ic_abastevo_logo
    }
    Surface(modifier, shape = RoundedCornerShape(18.dp), color = MaterialTheme.colorScheme.surfaceContainerLowest) {
        Image(painterResource(mark), contentDescription = null,
            modifier = Modifier.padding(8.dp))
    }
}
