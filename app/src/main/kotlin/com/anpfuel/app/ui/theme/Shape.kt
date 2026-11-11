package com.anpfuel.app.ui.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.unit.dp

/**
 * Soft radii of the black-chrome identity: 8 for small marks and menus, 12 for
 * buttons and fields, 16 for cards, 20 for large panels, 28 for sheets and
 * dialogs.
 */
internal val AbastevoShapes = Shapes(
    extraSmall = RoundedCornerShape(8.dp),
    small = RoundedCornerShape(12.dp),
    medium = RoundedCornerShape(16.dp),
    large = RoundedCornerShape(20.dp),
    extraLarge = RoundedCornerShape(28.dp),
)

object AbastevoShapeDefaults {

    /** Buttons and fields share the control radius; chips and floating fields are pills. */
    val control: Shape
        @Composable get() = MaterialTheme.shapes.small
}
