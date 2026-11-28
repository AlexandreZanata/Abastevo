package com.anpfuel.app.ui.components

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.isSpecified
import androidx.compose.ui.unit.sp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme

/**
 * Skeleton loading foundation (E0).
 *
 * Shimmer built only on Compose animation primitives over theme container
 * colors, so light/dark follow the app theme with no extra dependency.
 * Placeholders mirror the shared 12dp content-container shape and paddings
 * to avoid a layout jump when content arrives. The shimmer is decorative; the group
 * announces the existing loading label to accessibility services.
 */
@Composable
fun skeletonShimmer(cornerRadius: Dp = 8.dp): Modifier {
    val base = MaterialTheme.colorScheme.surfaceVariant
    val transition = rememberInfiniteTransition(label = "skeleton")
    val translate by transition.animateFloat(
        initialValue = 0f,
        targetValue = 900f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = 1300, easing = LinearEasing),
            repeatMode = RepeatMode.Restart,
        ),
        label = "skeleton-translate",
    )
    val brush = Brush.linearGradient(
        colors = listOf(
            base.copy(alpha = 0.55f),
            base,
            base.copy(alpha = 0.55f),
        ),
        start = Offset(x = translate - 260f, y = 0f),
        end = Offset(x = translate, y = 260f),
    )
    return Modifier
        .clip(RoundedCornerShape(cornerRadius))
        .background(brush)
}

@Composable
fun SkeletonGroup(
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    val description = stringResource(R.string.a11y_loading)
    Column(
        modifier = modifier.semantics { contentDescription = description },
        verticalArrangement = Arrangement.spacedBy(12.dp),
        content = content,
    )
}

@Composable
fun SkeletonLine(
    modifier: Modifier = Modifier,
    width: Dp? = null,
    height: Dp = 16.dp,
) {
    Box(
        modifier
            .then(if (width != null) Modifier.width(width) else Modifier.fillMaxWidth())
            .height(height)
            .then(skeletonShimmer()),
    )
}

/**
 * One line of [style] text: the box takes the style's exact line height, so a
 * skeleton row is as tall as the loaded row, and the bar inside reads as text.
 */
@Composable
fun SkeletonText(
    style: TextStyle,
    width: Dp,
    modifier: Modifier = Modifier,
) {
    val lineHeight = skeletonLineHeight(style)
    Box(
        modifier = modifier
            .width(width)
            .height(lineHeight),
        contentAlignment = Alignment.CenterStart,
    ) {
        Box(
            Modifier
                .fillMaxWidth()
                .height(lineHeight * TEXT_BAR_FRACTION)
                .then(skeletonShimmer(cornerRadius = 4.dp)),
        )
    }
}

/** Line height of [style] in dp, falling back to 1.4× the font size when unset. */
@Composable
fun skeletonLineHeight(style: TextStyle): Dp = with(LocalDensity.current) {
    val fontSize = style.fontSize.takeIf { it.isSpecified && it.isSp } ?: 14.sp
    val lineHeight = style.lineHeight
    when {
        lineHeight.isSp -> lineHeight.toDp()
        lineHeight.isEm -> fontSize.toDp() * lineHeight.value
        else -> fontSize.toDp() * 1.4f
    }
}

private const val TEXT_BAR_FRACTION = 0.7f

@Composable
fun SkeletonCard(
    modifier: Modifier = Modifier,
    height: Dp = 120.dp,
) {
    Box(
        modifier
            .fillMaxWidth()
            .height(height)
            .then(skeletonShimmer(cornerRadius = 12.dp)),
    )
}

@Composable
fun SkeletonButton(
    modifier: Modifier = Modifier,
) {
    Box(
        modifier
            .fillMaxWidth()
            .height(48.dp)
            .then(skeletonShimmer(cornerRadius = 12.dp)),
    )
}

@Preview(showBackground = true, name = "Skeleton light")
@Composable
private fun SkeletonLightPreview() {
    AnpFuelTheme(darkTheme = false, dynamicColor = false) {
        SkeletonGroup {
            SkeletonLine(width = 160.dp, height = 20.dp)
            SkeletonLine()
            SkeletonLine(width = 220.dp)
            SkeletonCard()
            SkeletonButton()
        }
    }
}

@Preview(showBackground = true, name = "Skeleton dark")
@Composable
private fun SkeletonDarkPreview() {
    AnpFuelTheme(darkTheme = true, dynamicColor = false) {
        SkeletonGroup {
            SkeletonLine(width = 160.dp, height = 20.dp)
            SkeletonLine()
            SkeletonLine(width = 220.dp)
            SkeletonCard()
            SkeletonButton()
        }
    }
}
