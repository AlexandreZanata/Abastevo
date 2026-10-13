package com.anpfuel.app.ui.components

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R

/**
 * README-banner brand hero: the supplied A mark plus the traced
 * uppercase wordmark on a white card (light) or a darker container
 * with a white wordmark (dark), following the approved README
 * artwork pattern. Shared as the default banner wherever a branded
 * header is needed (account, station profile).
 */
@Composable
fun AbastevoBrandBanner(
    modifier: Modifier = Modifier,
    showTagline: Boolean = true,
) {
    // Dark scheme: container darker than the page with a white wordmark;
    // light scheme keeps the original white card and navy lettering.
    // Luminance decides so manual theme overrides follow.
    val dark = MaterialTheme.colorScheme.surface.luminance() < 0.5f
    Column(
        modifier = modifier
            .widthIn(max = 520.dp)
            .fillMaxWidth()
            .background(
                if (dark) {
                    MaterialTheme.colorScheme.surfaceContainerLowest
                } else {
                    Color.White
                },
                shape = RoundedCornerShape(32.dp),
            )
            .padding(28.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Image(
            painter = painterResource(R.drawable.ic_abastevo_logo),
            contentDescription = null,
            modifier = Modifier.size(88.dp),
        )
        Image(
            painter = painterResource(R.drawable.abastevo_wordmark),
            contentDescription = stringResource(R.string.auth_brand_wordmark),
            modifier = Modifier.fillMaxWidth(0.8f).aspectRatio(415f / 50f),
            contentScale = ContentScale.Fit,
            colorFilter = if (dark) {
                ColorFilter.tint(Color.White)
            } else {
                null
            },
        )
        if (showTagline) {
            Text(
                text = stringResource(R.string.auth_brand_tagline),
                style = MaterialTheme.typography.titleMedium,
                color = if (dark) {
                    MaterialTheme.colorScheme.onSurface
                } else {
                    Color(0xFF0B1849)
                },
            )
        }
    }
}
