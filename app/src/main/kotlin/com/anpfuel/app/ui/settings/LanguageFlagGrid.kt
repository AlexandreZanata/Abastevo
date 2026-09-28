package com.anpfuel.app.ui.settings

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.anpfuel.app.locale.AppLocales
import com.anpfuel.app.locale.SupportedLocale
import com.anpfuel.app.ui.theme.AnpFuelTheme

private const val GRID_COLUMNS = 4

@Composable
fun LanguageFlagGrid(
    selectedLocaleTag: String,
    onLocaleSelected: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val rows = AppLocales.all.chunked(GRID_COLUMNS)

    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        rows.forEach { rowLocales ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                rowLocales.forEach { locale ->
                    LanguageFlagOption(
                        locale = locale,
                        selected = selectedLocaleTag == locale.localeTag,
                        onClick = { onLocaleSelected(locale.localeTag) },
                        modifier = Modifier.weight(1f),
                    )
                }
                repeat(GRID_COLUMNS - rowLocales.size) {
                    Spacer(modifier = Modifier.weight(1f))
                }
            }
        }
    }
}

@Composable
private fun LanguageFlagOption(
    locale: SupportedLocale,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val languageLabel = stringResource(locale.labelRes)
    val border = if (selected) {
        BorderStroke(2.dp, MaterialTheme.colorScheme.primary)
    } else {
        BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant)
    }

    Surface(
        onClick = onClick,
        modifier = modifier.semantics {
            contentDescription = languageLabel
        },
        shape = RoundedCornerShape(8.dp),
        border = border,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 4.dp, vertical = 6.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Image(
                painter = painterResource(locale.flagResId),
                contentDescription = null,
                contentScale = ContentScale.Fit,
                modifier = Modifier
                    .fillMaxWidth()
                    .aspectRatio(4f / 3f)
                    .padding(horizontal = 4.dp),
            )
            Text(
                text = languageLabel,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun LanguageFlagGridPreview() {
    AnpFuelTheme {
        LanguageFlagGrid(
            selectedLocaleTag = AppLocales.PORTUGUESE_BRAZIL_TAG,
            onLocaleSelected = {},
            modifier = Modifier.padding(16.dp),
        )
    }
}
