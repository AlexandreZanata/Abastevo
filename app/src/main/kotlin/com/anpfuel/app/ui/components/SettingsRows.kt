package com.anpfuel.app.ui.components

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.app.ui.theme.AnpContainerDefaults

private val RowIconSize = 22.dp

/** Divider inset to the row text, so icons read as one column. */
private val DividerInset = 16.dp + RowIconSize + 16.dp

/**
 * One settings group: a black section title over a white outlined card that
 * holds rows. Rows are separated with [SettingsDivider].
 */
@Composable
fun SettingsGroup(
    title: String?,
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        if (title != null) {
            Text(
                text = title,
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onSurface,
                modifier = Modifier
                    .padding(start = 4.dp)
                    .semantics { heading() },
            )
        }
        val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
        Card(
            modifier = Modifier.fillMaxWidth(),
            shape = AnpContainerDefaults.containerShape(),
            colors = CardDefaults.cardColors(containerColor = containerColor),
            border = AnpContainerDefaults.borderStroke(containerColor),
        ) {
            Column(content = content)
        }
    }
}

@Composable
fun SettingsDivider() {
    HorizontalDivider(
        modifier = Modifier.padding(start = DividerInset),
        color = MaterialTheme.colorScheme.outlineVariant,
    )
}

/**
 * Settings row: icon, title, optional subtitle and a trailing element. With
 * [onClick] the whole row is the touch target and shows a chevron unless
 * [trailing] replaces it. [destructive] paints icon and title in the error
 * color.
 */
@Composable
fun SettingsRow(
    icon: ImageVector,
    title: String,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
    onClick: (() -> Unit)? = null,
    enabled: Boolean = true,
    destructive: Boolean = false,
    trailing: (@Composable () -> Unit)? = null,
) {
    val tint = when {
        destructive -> MaterialTheme.colorScheme.error
        else -> MaterialTheme.colorScheme.onSurface
    }
    val clickModifier = if (onClick != null) {
        Modifier.clickable(enabled = enabled, role = Role.Button, onClick = onClick)
    } else {
        Modifier
    }
    SettingsRowLayout(
        icon = icon,
        title = title,
        subtitle = subtitle,
        tint = tint,
        modifier = modifier.then(clickModifier),
        enabled = enabled,
        trailing = trailing ?: if (onClick != null) {
            {
                Icon(
                    imageVector = Icons.Filled.ChevronRight,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onSurface,
                )
            }
        } else {
            null
        },
    )
}

/** Settings row with a switch; the whole row toggles it. */
@Composable
fun SettingsSwitchRow(
    icon: ImageVector,
    title: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
) {
    SettingsRowLayout(
        icon = icon,
        title = title,
        subtitle = subtitle,
        tint = MaterialTheme.colorScheme.onSurface,
        modifier = modifier.toggleable(value = checked, role = Role.Switch, onValueChange = onCheckedChange),
        enabled = true,
        trailing = { Switch(checked = checked, onCheckedChange = null) },
    )
}

@Composable
private fun SettingsRowLayout(
    icon: ImageVector,
    title: String,
    subtitle: String?,
    tint: androidx.compose.ui.graphics.Color,
    modifier: Modifier,
    enabled: Boolean,
    trailing: (@Composable () -> Unit)?,
) {
    val alpha = if (enabled) 1f else 0.38f
    Row(
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = 56.dp)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = tint.copy(alpha = alpha),
            modifier = Modifier.size(RowIconSize),
        )
        Column(
            modifier = Modifier.weight(1f),
            verticalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            Text(
                text = title,
                style = MaterialTheme.typography.bodyLarge,
                color = tint.copy(alpha = alpha),
            )
            if (subtitle != null) {
                Text(
                    text = subtitle,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = alpha),
                )
            }
        }
        trailing?.invoke()
    }
}
