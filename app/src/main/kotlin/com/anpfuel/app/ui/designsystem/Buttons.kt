package com.anpfuel.app.ui.designsystem

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.heightIn
import androidx.compose.material3.ButtonColors
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ButtonElevation
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.unit.dp
import com.anpfuel.app.ui.theme.AbastevoShapeDefaults
import com.anpfuel.app.ui.theme.AbastevoSpacing
import com.anpfuel.app.ui.theme.LocalAbastevoActionColors

/**
 * Design-system buttons: the Material 3 buttons with the app defaults
 * (control radius, 48dp minimum height, ink fill, accent text links).
 * Parameters match the Material components, so a caller can still override
 * any of them.
 */
object AbastevoButtonDefaults {

    val OutlinedBorderWidth = 1.5.dp

    /** Ink fill on light screens, white fill on black ones. */
    @Composable
    fun filledColors(): ButtonColors {
        val action = LocalAbastevoActionColors.current
        return ButtonDefaults.buttonColors(
            containerColor = action.container,
            contentColor = action.content,
        )
    }

    /** Secondary button: ink outline while enabled. */
    @Composable
    fun outlinedBorder(enabled: Boolean): BorderStroke = if (enabled) {
        BorderStroke(OutlinedBorderWidth, MaterialTheme.colorScheme.primary)
    } else {
        ButtonDefaults.outlinedButtonBorder(enabled = false)
    }

    /** Text links use the accent, so tappable text never reads as plain ink copy. */
    @Composable
    fun textColors(): ButtonColors =
        ButtonDefaults.textButtonColors(contentColor = MaterialTheme.colorScheme.secondary)
}

@Composable
fun Button(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    shape: Shape = AbastevoShapeDefaults.control,
    colors: ButtonColors = AbastevoButtonDefaults.filledColors(),
    elevation: ButtonElevation? = ButtonDefaults.buttonElevation(),
    border: BorderStroke? = null,
    contentPadding: PaddingValues = ButtonDefaults.ContentPadding,
    interactionSource: MutableInteractionSource? = null,
    content: @Composable RowScope.() -> Unit,
) {
    androidx.compose.material3.Button(
        onClick = onClick,
        modifier = modifier.heightIn(min = AbastevoSpacing.controlMinHeight),
        enabled = enabled,
        shape = shape,
        colors = colors,
        elevation = elevation,
        border = border,
        contentPadding = contentPadding,
        interactionSource = interactionSource,
        content = content,
    )
}

@Composable
fun OutlinedButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    shape: Shape = AbastevoShapeDefaults.control,
    colors: ButtonColors = ButtonDefaults.outlinedButtonColors(),
    elevation: ButtonElevation? = null,
    border: BorderStroke? = AbastevoButtonDefaults.outlinedBorder(enabled),
    contentPadding: PaddingValues = ButtonDefaults.ContentPadding,
    interactionSource: MutableInteractionSource? = null,
    content: @Composable RowScope.() -> Unit,
) {
    androidx.compose.material3.OutlinedButton(
        onClick = onClick,
        modifier = modifier.heightIn(min = AbastevoSpacing.controlMinHeight),
        enabled = enabled,
        shape = shape,
        colors = colors,
        elevation = elevation,
        border = border,
        contentPadding = contentPadding,
        interactionSource = interactionSource,
        content = content,
    )
}

@Composable
fun TextButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    shape: Shape = AbastevoShapeDefaults.control,
    colors: ButtonColors = AbastevoButtonDefaults.textColors(),
    elevation: ButtonElevation? = null,
    border: BorderStroke? = null,
    contentPadding: PaddingValues = ButtonDefaults.TextButtonContentPadding,
    interactionSource: MutableInteractionSource? = null,
    content: @Composable RowScope.() -> Unit,
) {
    androidx.compose.material3.TextButton(
        onClick = onClick,
        modifier = modifier,
        enabled = enabled,
        shape = shape,
        colors = colors,
        elevation = elevation,
        border = border,
        contentPadding = contentPadding,
        interactionSource = interactionSource,
        content = content,
    )
}
