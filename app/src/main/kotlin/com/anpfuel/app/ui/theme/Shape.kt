package com.anpfuel.app.ui.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.unit.dp

/**
 * Landing radii on the Material roles: `--radius-sm` (8) for controls,
 * `--radius-md` (12) for cards, 16 for large panels, 24 for sheets and dialogs.
 */
internal val AbastevoShapes = Shapes(
    extraSmall = RoundedCornerShape(8.dp),
    small = RoundedCornerShape(8.dp),
    medium = RoundedCornerShape(12.dp),
    large = RoundedCornerShape(16.dp),
    extraLarge = RoundedCornerShape(24.dp),
)

object AbastevoShapeDefaults {

    /** Buttons share the landing button radius; chips and floating fields are pills. */
    val control: Shape
        @Composable get() = MaterialTheme.shapes.small
}
