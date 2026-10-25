package com.anpfuel.app.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import com.anpfuel.app.ui.theme.AnpContainerDefaults

/**
 * Shared full-width action button: card with leading icon, label and
 * trailing chevron. One implementation backs settings entries and form
 * actions alike, so press feedback and shape stay identical everywhere.
 */
@Composable
fun ActionCardButton(
    label: String,
    icon: ImageVector,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    style: ActionCardStyle = ActionCardStyle.Outlined,
    enabled: Boolean = true,
) {
    val containerColor = when (style) {
        ActionCardStyle.Primary -> MaterialTheme.colorScheme.secondaryContainer
        ActionCardStyle.Outlined -> Color.Transparent
        ActionCardStyle.Destructive -> Color.Transparent
    }
    val contentColor = when (style) {
        ActionCardStyle.Primary -> MaterialTheme.colorScheme.onSecondaryContainer
        ActionCardStyle.Outlined -> MaterialTheme.colorScheme.onSurface
        ActionCardStyle.Destructive -> MaterialTheme.colorScheme.error
    }
    val border = when (style) {
        ActionCardStyle.Primary -> null
        ActionCardStyle.Outlined ->
            BorderStroke(
                1.dp,
                MaterialTheme.colorScheme.outline,
            )
        ActionCardStyle.Destructive ->
            BorderStroke(
                1.dp,
                MaterialTheme.colorScheme.error,
            )
    }
    Card(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(
            containerColor = containerColor,
            contentColor = contentColor,
        ),
        border = border,
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Icon(
                imageVector = icon,
                contentDescription = null,
            )
            Text(
                text = label,
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.weight(1f),
            )
            Icon(
                imageVector = Icons.Default.ChevronRight,
                contentDescription = null,
            )
        }
    }
}

enum class ActionCardStyle {
    Primary,
    Outlined,
    Destructive,
}
