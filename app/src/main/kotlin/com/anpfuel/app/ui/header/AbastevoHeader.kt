package com.anpfuel.app.ui.header

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.LocationOn
import androidx.compose.material.icons.outlined.CalendarMonth
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
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
import com.anpfuel.app.ui.theme.AbastevoSpacing
import com.anpfuel.app.ui.theme.LocalAbastevoChromeColors
import com.anpfuel.app.ui.weekpicker.SurveyWeekChipAction
import com.anpfuel.domain.valueobject.SurveyWeek

private val FieldHeight = 52.dp
private val WeekChipHeight = 40.dp
private val LogoSize = 32.dp

/**
 * Shared brand header on the solid black chrome, under the status bar:
 * logo and name with notification and profile buttons. When [locationLabel]
 * is set, one pill field shows the city on the left and, when [onWeekChanged]
 * is set, the survey week as a light chip on the right.
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
    val chrome = LocalAbastevoChromeColors.current
    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(chrome.container)
            .statusBarsPadding(),
    ) {
        Column(
            modifier = Modifier.padding(
                start = AbastevoSpacing.screenGutter,
                end = AbastevoSpacing.sm,
                top = AbastevoSpacing.xs,
                bottom = AbastevoSpacing.md,
            ),
            verticalArrangement = Arrangement.spacedBy(AbastevoSpacing.sm),
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(AbastevoSpacing.sm),
            ) {
                Image(
                    painter = painterResource(R.drawable.ic_abastevo_logo),
                    contentDescription = null,
                    modifier = Modifier.size(LogoSize),
                )
                Text(
                    text = stringResource(R.string.app_name).replaceFirstChar { it.titlecase() },
                    modifier = Modifier.weight(1f),
                    style = MaterialTheme.typography.headlineSmall,
                    fontWeight = FontWeight.ExtraBold,
                    color = chrome.content,
                    maxLines = 1,
                )
                HeaderIconButton(
                    icon = Icons.Outlined.Notifications,
                    contentDescription = stringResource(R.string.settings_notifications_section),
                    onClick = onNotificationsClick,
                )
                HeaderIconButton(
                    icon = Icons.Outlined.Person,
                    contentDescription = stringResource(R.string.nav_profile),
                    onClick = onProfileClick,
                )
            }
            if (locationLabel != null) {
                Row(
                    modifier = Modifier
                        .padding(end = AbastevoSpacing.sm)
                        .fillMaxWidth()
                        .height(FieldHeight)
                        .clip(CircleShape)
                        .background(chrome.control),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Row(
                        modifier = Modifier
                            .weight(1f)
                            .fillMaxHeight()
                            .clickable(role = Role.Button, onClick = onLocationClick)
                            .padding(start = AbastevoSpacing.lg, end = AbastevoSpacing.sm),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(AbastevoSpacing.sm),
                    ) {
                        Icon(
                            imageVector = Icons.Filled.LocationOn,
                            contentDescription = null,
                            tint = chrome.accent,
                            modifier = Modifier.size(20.dp),
                        )
                        Text(
                            text = locationLabel,
                            modifier = Modifier.weight(1f, fill = false),
                            style = MaterialTheme.typography.titleMedium,
                            fontWeight = FontWeight.Bold,
                            color = chrome.content,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                        Icon(
                            imageVector = Icons.Filled.KeyboardArrowDown,
                            contentDescription = null,
                            tint = chrome.contentMuted,
                            modifier = Modifier.size(20.dp),
                        )
                    }
                    onWeekChanged?.let { HeaderWeekAction(onWeekChanged = it) }
                }
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
    val chrome = LocalAbastevoChromeColors.current
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
    // The whole right end of the field is the touch target; the chip is its visual.
    Box(
        modifier = modifier
            .fillMaxHeight()
            .clickable(role = Role.Button, onClick = onClick)
            .semantics { contentDescription = description }
            .padding(start = AbastevoSpacing.xs, end = 6.dp),
        contentAlignment = Alignment.Center,
    ) {
        Row(
            modifier = Modifier
                .height(WeekChipHeight)
                .background(chrome.selected, CircleShape)
                .padding(horizontal = AbastevoSpacing.md),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Icon(
                imageVector = Icons.Outlined.CalendarMonth,
                contentDescription = null,
                tint = chrome.onSelected,
                modifier = Modifier.size(18.dp),
            )
            Text(
                text = label,
                style = MaterialTheme.typography.labelLarge,
                color = chrome.onSelected,
                maxLines = 1,
            )
        }
    }
}

@Composable
private fun HeaderIconButton(icon: ImageVector, contentDescription: String, onClick: () -> Unit) {
    val chrome = LocalAbastevoChromeColors.current
    IconButton(
        onClick = onClick,
        colors = IconButtonDefaults.iconButtonColors(
            containerColor = chrome.control,
            contentColor = chrome.content,
        ),
    ) {
        Icon(
            imageVector = icon,
            contentDescription = contentDescription,
            modifier = Modifier.size(22.dp),
        )
    }
}
