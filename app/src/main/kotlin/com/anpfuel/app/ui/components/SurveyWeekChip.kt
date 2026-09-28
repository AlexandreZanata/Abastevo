package com.anpfuel.app.ui.components

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AssistChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.mapper.SurveyWeekFormatter
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.valueobject.SurveyWeek

@Composable
fun SurveyWeekChip(
    surveyWeek: SurveyWeek,
    isLatest: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val locale = LocalConfiguration.current.locales[0]
    val weekLabel = SurveyWeekFormatter.formatRangeCompact(surveyWeek, locale)
    val accessibilityWeekLabel = SurveyWeekFormatter.formatRange(surveyWeek, locale)
    val accessibilityLabel = stringResource(
        if (isLatest) {
            R.string.a11y_survey_week_chip_latest
        } else {
            R.string.a11y_survey_week_chip
        },
        accessibilityWeekLabel,
    )

    AssistChip(
        onClick = onClick,
        modifier = modifier.semantics {
            contentDescription = accessibilityLabel
        },
        label = {
            Row {
                Text(
                    text = weekLabel,
                    style = MaterialTheme.typography.labelLarge,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                if (isLatest) {
                    Text(
                        text = stringResource(R.string.survey_week_chip_latest_suffix),
                        style = MaterialTheme.typography.labelSmall,
                        modifier = Modifier.padding(start = 4.dp),
                        maxLines = 1,
                    )
                }
            }
        },
    )
}

@Preview(showBackground = true)
@Composable
private fun SurveyWeekChipPreview() {
    AnpFuelTheme {
        SurveyWeekChip(
            surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
            isLatest = true,
            onClick = {},
            modifier = Modifier.padding(16.dp),
        )
    }
}
