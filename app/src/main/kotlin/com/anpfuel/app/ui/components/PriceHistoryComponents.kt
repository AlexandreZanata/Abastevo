package com.anpfuel.app.ui.components

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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.mapper.SurveyWeekFormatter
import com.anpfuel.app.ui.accessibility.accessibilityDescription
import com.anpfuel.app.ui.model.HistoryEntryUiModel
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.valueobject.SurveyWeek
import java.math.BigDecimal
import java.math.RoundingMode
import java.util.Locale

@Composable
fun PriceHistoryEntryRow(
    weekLabel: String,
    averageFormatted: String,
    modifier: Modifier = Modifier,
) {
    Card(
        modifier = modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        ),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(16.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = weekLabel,
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurface,
                modifier = Modifier.weight(1f),
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = averageFormatted,
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.primary,
                modifier = Modifier.padding(start = 8.dp),
                maxLines = 1,
                softWrap = false,
            )
        }
    }
}

@Composable
fun PriceHistoryTrendChart(
    entries: List<HistoryEntryUiModel>,
    modifier: Modifier = Modifier,
    locale: Locale = LocalConfiguration.current.locales[0],
) {
    val chartEntries = remember(entries) {
        entries
            .sortedBy { it.surveyWeek.startDate }
            .filter { it.averageValue != null }
    }
    val values = chartEntries.mapNotNull { it.averageValue }
    if (values.size < 2) {
        return
    }

    var selectedBarIndex by remember(chartEntries) { mutableIntStateOf(-1) }

    val maxValue = values.maxOf { it }
    val minValue = values.minOf { it }
    val range = maxValue.subtract(minValue).takeIf { it.signum() > 0 } ?: BigDecimal.ONE

    Card(
        modifier = modifier
            .fillMaxWidth()
            .accessibilityDescription(stringResource(R.string.a11y_price_trend_chart)),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerHighest,
        ),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(120.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.Bottom,
            ) {
                chartEntries.forEachIndexed { index, entry ->
                    val value = entry.averageValue ?: return@forEachIndexed

                    val normalized = value.subtract(minValue)
                        .divide(range, 4, RoundingMode.HALF_UP)
                        .toFloat()
                        .coerceIn(0f, 1f)
                    val heightFraction = 0.15f + (normalized * 0.85f)
                    val weekLabel = SurveyWeekFormatter.formatRangeCompact(entry.surveyWeek, locale)
                    val isSelected = selectedBarIndex == index
                    val barColor = if (isSelected) {
                        MaterialTheme.colorScheme.primary
                    } else if (selectedBarIndex >= 0) {
                        MaterialTheme.colorScheme.primary.copy(alpha = 0.45f)
                    } else {
                        MaterialTheme.colorScheme.primary
                    }

                    Box(
                        modifier = Modifier
                            .weight(1f)
                            .fillMaxHeight()
                            .semantics {
                                contentDescription = weekLabel
                                role = Role.Button
                            }
                            .clickable {
                                selectedBarIndex = if (isSelected) -1 else index
                            },
                        contentAlignment = Alignment.BottomCenter,
                    ) {
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .fillMaxHeight(heightFraction)
                                .padding(horizontal = 2.dp)
                                .clip(RoundedCornerShape(topStart = 4.dp, topEnd = 4.dp))
                                .background(barColor),
                        )
                    }
                }
            }

            if (selectedBarIndex >= 0) {
                val selectedEntry = chartEntries[selectedBarIndex]
                val weekLabel = SurveyWeekFormatter.formatRangeCompact(selectedEntry.surveyWeek, locale)
                val priceLabel = selectedEntry.averageFormatted
                    ?: stringResource(R.string.prices_not_available)

                Text(
                    text = stringResource(R.string.history_chart_bar_legend, weekLabel, priceLabel),
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.onSurface,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth(),
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun PriceHistoryEntryRowPreview() {
    AnpFuelTheme {
        PriceHistoryEntryRow(
            weekLabel = "Jun 7 – Jun 13, 2026",
            averageFormatted = "R$ 5,89",
        )
    }
}

@Preview(showBackground = true)
@Composable
private fun PriceHistoryTrendChartPreview() {
    AnpFuelTheme {
        PriceHistoryTrendChart(
            entries = listOf(
                historyEntry("2026-05-31", "2026-06-06", "5.50"),
                historyEntry("2026-06-07", "2026-06-13", "5.89"),
                historyEntry("2026-06-14", "2026-06-20", "5.72"),
            ),
        )
    }
}

private fun historyEntry(start: String, end: String, price: String): HistoryEntryUiModel =
    HistoryEntryUiModel(
        surveyWeek = SurveyWeek.fromIsoDates(start, end),
        averageFormatted = "R$ $price",
        averageValue = BigDecimal(price),
    )
