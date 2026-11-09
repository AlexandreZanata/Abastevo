package com.anpfuel.app.ui.header

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.outlined.CalendarMonth
import androidx.compose.material.icons.outlined.LocationOn
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.mapper.SurveyWeekFormatter
import com.anpfuel.app.ui.theme.BrandNavy
import com.anpfuel.app.ui.weekpicker.SurveyWeekChipAction
import com.anpfuel.domain.valueobject.SurveyWeek

/** Brand navy of the header. The same gradient is used in light and dark themes. */
private object HeaderColors {
    val NavyTop = Color(0xFF061433)
    val NavyBottom = BrandNavy
    val Content = Color.White
    val ContentMuted = Color.White.copy(alpha = 0.72f)
    val ButtonFill = Color.White.copy(alpha = 0.08f)
    val Outline = Color.White.copy(alpha = 0.28f)
}

private val HeaderShape = RoundedCornerShape(bottomStart = 28.dp, bottomEnd = 28.dp)
private val ControlHeight = 52.dp

/**
 * Shared brand header: logo, name and tagline with notification and profile
 * buttons. When [locationLabel] is set, a single outlined field shows the city
 * on the left and the survey week picker on the right when [onWeekChanged] is set.
 */
@Composable
fun AbastevoHeader(
    onNotificationsClick: () -> Unit,
    onProfileClick: () -> Unit,
    modifier: Modifier = Modifier,
    locationLabel: String? = null,
    onLocationClick: () -> Unit = {},
    onWeekChanged: (() -> Unit)? = null,
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(
                Brush.verticalGradient(listOf(HeaderColors.NavyTop, HeaderColors.NavyBottom)),
                HeaderShape,
            )
            .statusBarsPadding()
            .padding(start = 16.dp, end = 16.dp, top = 8.dp, bottom = 18.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Image(
                painter = painterResource(R.drawable.ic_abastevo_logo),
                contentDescription = null,
                modifier = Modifier.size(48.dp),
            )
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = stringResource(R.string.app_name).replaceFirstChar { it.titlecase() },
                    style = MaterialTheme.typography.headlineSmall,
                    fontWeight = FontWeight.Bold,
                    color = HeaderColors.Content,
                    maxLines = 1,
                )
                Text(
                    text = stringResource(R.string.header_tagline),
                    style = MaterialTheme.typography.bodyMedium,
                    color = HeaderColors.ContentMuted,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            HeaderIconButton(
                icon = Icons.Outlined.Notifications,
                contentDescription = stringResource(R.string.settings_notifications_section),
                onClick = onNotificationsClick,
            )
            HeaderIconButton(
                icon = Icons.Filled.Person,
                contentDescription = stringResource(R.string.nav_profile),
                onClick = onProfileClick,
            )
        }
        if (locationLabel != null) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(ControlHeight)
                    .clip(CircleShape)
                    .background(HeaderColors.ButtonFill)
                    .border(1.dp, HeaderColors.Outline, CircleShape),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Row(
                    modifier = Modifier
                        .weight(1f)
                        .fillMaxHeight()
                        .clickable(role = Role.Button, onClick = onLocationClick)
                        .padding(start = 16.dp, end = 8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Icon(
                        imageVector = Icons.Outlined.LocationOn,
                        contentDescription = null,
                        tint = HeaderColors.Content,
                        modifier = Modifier.size(22.dp),
                    )
                    Text(
                        text = locationLabel,
                        modifier = Modifier.weight(1f, fill = false),
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.Bold,
                        color = HeaderColors.Content,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    Icon(
                        imageVector = Icons.Filled.KeyboardArrowDown,
                        contentDescription = null,
                        tint = HeaderColors.Content,
                        modifier = Modifier.size(22.dp),
                    )
                }
                onWeekChanged?.let { HeaderWeekAction(onWeekChanged = it) }
            }
        }
    }
}

/** Survey week shown inside the header field; opens the week picker. */
@Composable
fun HeaderWeekAction(onWeekChanged: () -> Unit) {
    SurveyWeekChipAction(
        onWeekChanged = onWeekChanged,
        hasTrailingAction = true,
        trigger = { surveyWeek, isLatest, isStale, onClick, modifier ->
            HeaderWeekLabel(surveyWeek, isLatest, isStale, onClick, modifier)
        },
    )
}

@Composable
private fun HeaderWeekLabel(
    surveyWeek: SurveyWeek,
    isLatest: Boolean,
    isStale: Boolean,
    onClick: () -> Unit,
    modifier: Modifier,
) {
    val locale = LocalConfiguration.current.locales[0]
    val label = SurveyWeekFormatter.formatRangeCompact(surveyWeek, locale)
    val description = stringResource(
        when {
            isLatest && !isStale -> R.string.a11y_survey_week_chip_latest
            isLatest -> R.string.a11y_survey_week_chip_stale_latest
            else -> R.string.a11y_survey_week_chip
        },
        SurveyWeekFormatter.formatRange(surveyWeek, locale),
    )
    Row(
        modifier = modifier
            .fillMaxHeight()
            .clickable(role = Role.Button, onClick = onClick)
            .semantics { contentDescription = description }
            .padding(start = 8.dp, end = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.CalendarMonth,
            contentDescription = null,
            tint = HeaderColors.Content,
            modifier = Modifier.size(22.dp),
        )
        Text(
            text = label,
            style = MaterialTheme.typography.titleSmall,
            color = HeaderColors.Content,
            maxLines = 1,
        )
    }
}

@Composable
private fun HeaderIconButton(icon: ImageVector, contentDescription: String, onClick: () -> Unit) {
    Box(
        contentAlignment = Alignment.Center,
        modifier = Modifier
            .size(46.dp)
            .clip(CircleShape)
            .background(HeaderColors.ButtonFill)
            .border(1.dp, HeaderColors.Outline, CircleShape)
            .clickable(role = Role.Button, onClick = onClick)
            .semantics { this.contentDescription = contentDescription },
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = HeaderColors.Content,
            modifier = Modifier.size(24.dp),
        )
    }
}
