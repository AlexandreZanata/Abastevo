package com.anpfuel.app.ui.designsystem

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.AssistChipDefaults
import androidx.compose.material3.ChipColors
import androidx.compose.material3.ChipElevation
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SegmentedButtonColors
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SelectableChipColors
import androidx.compose.material3.SelectableChipElevation
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Shape

/**
 * A selected option is always solid ink with an inverse label, in chips and
 * segmented controls alike, so selection never depends on telling two tints
 * apart. Chips are pills; buttons keep the 8dp control radius.
 */
object AbastevoSelectionDefaults {

    val chipShape: Shape = CircleShape

    @Composable
    fun filterChipColors(): SelectableChipColors {
        val scheme = MaterialTheme.colorScheme
        return FilterChipDefaults.filterChipColors(
            selectedContainerColor = scheme.primary,
            selectedLabelColor = scheme.onPrimary,
            selectedLeadingIconColor = scheme.onPrimary,
            selectedTrailingIconColor = scheme.onPrimary,
        )
    }

    @Composable
    fun segmentedButtonColors(): SegmentedButtonColors {
        val scheme = MaterialTheme.colorScheme
        return SegmentedButtonDefaults.colors(
            activeContainerColor = scheme.primary,
            activeContentColor = scheme.onPrimary,
            activeBorderColor = scheme.primary,
            inactiveContainerColor = scheme.surface,
        )
    }
}

@Composable
fun FilterChip(
    selected: Boolean,
    onClick: () -> Unit,
    label: @Composable () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    leadingIcon: @Composable (() -> Unit)? = null,
    trailingIcon: @Composable (() -> Unit)? = null,
    shape: Shape = AbastevoSelectionDefaults.chipShape,
    colors: SelectableChipColors = AbastevoSelectionDefaults.filterChipColors(),
    elevation: SelectableChipElevation? = FilterChipDefaults.filterChipElevation(),
    border: BorderStroke? = FilterChipDefaults.filterChipBorder(enabled, selected),
    interactionSource: MutableInteractionSource? = null,
) {
    androidx.compose.material3.FilterChip(
        selected = selected,
        onClick = onClick,
        label = label,
        modifier = modifier,
        enabled = enabled,
        leadingIcon = leadingIcon,
        trailingIcon = trailingIcon,
        shape = shape,
        colors = colors,
        elevation = elevation,
        border = border,
        interactionSource = interactionSource,
    )
}

@Composable
fun AssistChip(
    onClick: () -> Unit,
    label: @Composable () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    leadingIcon: @Composable (() -> Unit)? = null,
    trailingIcon: @Composable (() -> Unit)? = null,
    shape: Shape = AbastevoSelectionDefaults.chipShape,
    colors: ChipColors = AssistChipDefaults.assistChipColors(),
    elevation: ChipElevation? = AssistChipDefaults.assistChipElevation(),
    border: BorderStroke? = AssistChipDefaults.assistChipBorder(enabled),
    interactionSource: MutableInteractionSource? = null,
) {
    androidx.compose.material3.AssistChip(
        onClick = onClick,
        label = label,
        modifier = modifier,
        enabled = enabled,
        leadingIcon = leadingIcon,
        trailingIcon = trailingIcon,
        shape = shape,
        colors = colors,
        elevation = elevation,
        border = border,
        interactionSource = interactionSource,
    )
}
