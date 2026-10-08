package com.anpfuel.app.ui.components

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.scale
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch

/**
 * Frontend-only toggle icon with a spring pulse on every tap.
 *
 * Selection state lives in the caller (`remember`); tapping never
 * navigates and never reaches the backend. Tint reflects the toggle
 * state while the scale pulse is purely cosmetic.
 */
@Composable
fun BouncyToggleIcon(
    selected: Boolean,
    onToggle: () -> Unit,
    icon: ImageVector,
    contentDescription: String,
    modifier: Modifier = Modifier,
    selectedTint: Color = MaterialTheme.colorScheme.primary,
    unselectedTint: Color = MaterialTheme.colorScheme.onSurfaceVariant,
) {
    val pulse = remember { Animatable(1f) }
    val scope = rememberCoroutineScope()
    IconButton(
        onClick = {
            onToggle()
            scope.launch {
                pulse.animateTo(1.35f, spring(
                    stiffness = Spring.StiffnessMediumLow,
                    dampingRatio = Spring.DampingRatioMediumBouncy,
                ))
                pulse.animateTo(1f, spring(
                    stiffness = Spring.StiffnessMedium,
                    dampingRatio = Spring.DampingRatioMediumBouncy,
                ))
            }
        },
        modifier = modifier.size(40.dp),
    ) {
        Icon(
            imageVector = icon,
            contentDescription = contentDescription,
            tint = if (selected) selectedTint else unselectedTint,
            modifier = Modifier.scale(pulse.value).size(22.dp),
        )
    }
}
