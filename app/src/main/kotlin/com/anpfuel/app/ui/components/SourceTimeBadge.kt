package com.anpfuel.app.ui.components

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.DateRange
import androidx.compose.material.icons.filled.HelpOutline
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R

/**
 * P19-T03 price source/time badge. Presentation only: the caller passes the
 * precomputed [detail] (observation time or survey date); no domain rules
 * live in this composable. Source is always text plus icon, never color
 * alone, with a screen-reader description.
 */
enum class PriceSourceKind {
    COMMUNITY,
    ANP_DATED,
    UNKNOWN,
}

object SourceTimeBadgeLabels {
    fun labelRes(kind: PriceSourceKind): Int = when (kind) {
        PriceSourceKind.COMMUNITY -> R.string.source_badge_community
        PriceSourceKind.ANP_DATED -> R.string.source_badge_anp_dated
        PriceSourceKind.UNKNOWN -> R.string.source_badge_unknown
    }

    fun a11yRes(kind: PriceSourceKind): Int = when (kind) {
        PriceSourceKind.COMMUNITY -> R.string.a11y_source_badge_community
        PriceSourceKind.ANP_DATED -> R.string.a11y_source_badge_anp_dated
        PriceSourceKind.UNKNOWN -> R.string.a11y_source_badge_unknown
    }
}

@Composable
fun SourceTimeBadge(
    kind: PriceSourceKind,
    detail: String,
    modifier: Modifier = Modifier,
) {
    val label = when (kind) {
        PriceSourceKind.COMMUNITY -> stringResource(SourceTimeBadgeLabels.labelRes(kind))
        PriceSourceKind.ANP_DATED -> stringResource(SourceTimeBadgeLabels.labelRes(kind), detail)
        PriceSourceKind.UNKNOWN -> stringResource(SourceTimeBadgeLabels.labelRes(kind))
    }
    val icon = when (kind) {
        PriceSourceKind.COMMUNITY -> Icons.Filled.Check
        PriceSourceKind.ANP_DATED -> Icons.Filled.DateRange
        PriceSourceKind.UNKNOWN -> Icons.Filled.HelpOutline
    }
    val description = stringResource(SourceTimeBadgeLabels.a11yRes(kind), detail)

    Surface(
        shape = MaterialTheme.shapes.small,
        tonalElevation = 1.dp,
        modifier = modifier.semantics { contentDescription = description },
    ) {
        Row(modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp)) {
            Icon(imageVector = icon, contentDescription = null)
            Text(
                text = label,
                style = MaterialTheme.typography.labelLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(start = 4.dp),
            )
        }
    }
}
