package com.anpfuel.app.ui.stations

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AbastevoActionBlue
import com.anpfuel.app.ui.theme.AbastevoCommunityGreen
import com.anpfuel.app.ui.theme.ColorTokens
import com.anpfuel.domain.profile.StationArtworkAsset
import com.anpfuel.domain.profile.StationArtwork

/** SVG-derived mark and responsive wordmark; local default artwork is not a business badge. */
@Composable
internal fun StationArtworkHeader(artwork: StationArtwork, modifier: Modifier = Modifier) {
    val dark = MaterialTheme.colorScheme.surface.luminance() < 0.5f
    val blue = if (dark) ColorTokens.BlueLight else AbastevoActionBlue
    val green = if (dark) ColorTokens.GreenLight else AbastevoCommunityGreen
    Column(modifier, verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Box(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(28.dp)).background(
                Brush.linearGradient(listOf(MaterialTheme.colorScheme.primaryContainer,
                    MaterialTheme.colorScheme.secondaryContainer, MaterialTheme.colorScheme.surfaceContainerHigh))),
        ) {
            Column(Modifier.padding(28.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    StationArtworkIcon(Modifier.size(48.dp), artwork.copy(icon = artwork.banner))
                    Text(buildAnnotatedString {
                        withStyle(SpanStyle(color = blue)) { append("abaste") }
                        withStyle(SpanStyle(color = green)) { append("vo") }
                    }, modifier = Modifier.weight(1f), style = MaterialTheme.typography.headlineLarge.copy(fontWeight = FontWeight.ExtraBold, letterSpacing = (-1).sp))
                }
                Text(stringResource(R.string.station_page_cover_tagline), style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.widthIn(max = 400.dp))
            }
        }
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
