package com.anpfuel.app.ui.premium

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.app.ui.theme.PremiumColors

/** Shared Premium entry used by Perfil and Gerenciar conta. */
@Composable
fun PremiumEntryCard(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    compact: Boolean = false,
) {
    val palette = PremiumColors.current()
    Card(
        onClick = onClick,
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(if (compact) 18.dp else 22.dp),
        colors = CardDefaults.cardColors(
            containerColor = palette.container,
            contentColor = palette.content,
        ),
        border = BorderStroke(1.dp, palette.border),
    ) {
        Row(
            modifier = Modifier.padding(
                horizontal = 16.dp,
                vertical = if (compact) 12.dp else 14.dp,
            ),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(
                painter = painterResource(R.drawable.ic_premium_crown),
                contentDescription = null,
                tint = palette.accent,
                modifier = Modifier.size(24.dp),
            )
            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(2.dp),
            ) {
                Text(
                    text = stringResource(
                        if (compact) R.string.premium_section_title else R.string.premium_entry_title,
                    ),
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold,
                )
                if (!compact) {
                    Text(
                        text = stringResource(R.string.premium_entry_subtitle),
                        style = MaterialTheme.typography.bodySmall,
                        color = palette.accent,
                    )
                }
            }
            Icon(
                imageVector = Icons.Default.ChevronRight,
                contentDescription = null,
                tint = palette.accent,
            )
        }
    }
}

@Preview(showBackground = true, name = "Premium entry light")
@Composable
private fun PremiumEntryLightPreview() {
    AnpFuelTheme(darkTheme = false, dynamicColor = false) {
        PremiumEntryCard(onClick = {})
    }
}

@Preview(showBackground = true, name = "Premium entry dark")
@Composable
private fun PremiumEntryDarkPreview() {
    AnpFuelTheme(darkTheme = true, dynamicColor = false) {
        PremiumEntryCard(onClick = {})
    }
}

@Preview(showBackground = true, name = "Premium entry compact light")
@Composable
private fun PremiumEntryCompactLightPreview() {
    AnpFuelTheme(darkTheme = false, dynamicColor = false) {
        PremiumEntryCard(onClick = {}, compact = true)
    }
}
