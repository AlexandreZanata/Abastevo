package com.anpfuel.app.ui.forum

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material.icons.filled.ThumbDown
import androidx.compose.material.icons.outlined.FavoriteBorder
import androidx.compose.material.icons.outlined.ThumbDown
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.scale
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.anpfuel.app.ui.theme.LocalAnpDarkTheme

/**
 * Conversation pieces in the spirit of X: like and dislike toggles with an
 * animated count, the line that links a reply chain under the avatars and
 * the floating pill announcing new replies that arrived live.
 */

/** Pink of an active like, as on X; the darker tone keeps 4.5:1 on white. */
@Composable
internal fun forumLikeColor(): Color =
    if (LocalAnpDarkTheme.current) Color(0xFFF91880) else Color(0xFFC8105F)

/**
 * Like/dislike toggle with its count. The icon pops when it turns on and the
 * count rolls up or down, so the change reads at a glance.
 */
@Composable
internal fun ForumReactionToggle(
    active: Boolean,
    count: Long,
    activeIcon: ImageVector,
    inactiveIcon: ImageVector,
    activeColor: Color,
    label: String,
    stateLabel: String,
    onToggle: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
) {
    val inactiveColor = MaterialTheme.colorScheme.onSurfaceVariant
    val tint = if (active) activeColor else inactiveColor
    val pop by animateFloatAsState(
        targetValue = if (active) 1f else 0.92f,
        animationSpec = spring(dampingRatio = Spring.DampingRatioMediumBouncy, stiffness = Spring.StiffnessMedium),
        label = "reaction-pop",
    )
    Row(
        modifier = modifier
            .height(40.dp)
            .toggleable(value = active, enabled = enabled, role = Role.Checkbox, onValueChange = { onToggle() })
            .semantics {
                contentDescription = label
                stateDescription = stateLabel
            }
            .padding(horizontal = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Icon(
            imageVector = if (active) activeIcon else inactiveIcon,
            contentDescription = null,
            tint = tint,
            modifier = Modifier
                .size(19.dp)
                .scale(pop),
        )
        AnimatedCount(count = count, color = tint)
    }
}

@Composable
private fun AnimatedCount(count: Long, color: Color) {
    AnimatedContent(
        targetState = count,
        transitionSpec = {
            val up = targetState > initialState
            (slideInVertically { h -> if (up) h else -h } + fadeIn()) togetherWith
                (slideOutVertically { h -> if (up) -h else h } + fadeOut())
        },
        label = "reaction-count",
    ) { value ->
        Text(
            text = if (value > 0) formatCompactCount(value) else "",
            style = MaterialTheme.typography.labelMedium,
            color = color,
        )
    }
}

/** 999, 1,2 mil, 12 mil, 1,2 mi in the app's locale style. */
internal fun formatCompactCount(value: Long): String = when {
    value < 1_000 -> value.toString()
    value < 1_000_000 -> compact(value / 1_000.0, "mil")
    else -> compact(value / 1_000_000.0, "mi")
}

private fun compact(scaled: Double, suffix: String): String {
    val rounded = if (scaled < 10) Math.floor(scaled * 10) / 10 else Math.floor(scaled)
    val number = if (rounded % 1.0 == 0.0) rounded.toLong().toString() else rounded.toString().replace('.', ',')
    return "$number $suffix"
}

@Composable
internal fun ForumLikeToggle(
    liked: Boolean,
    count: Long,
    label: String,
    stateLabel: String,
    onToggle: () -> Unit,
    enabled: Boolean = true,
) = ForumReactionToggle(
    active = liked,
    count = count,
    activeIcon = Icons.Filled.Favorite,
    inactiveIcon = Icons.Outlined.FavoriteBorder,
    activeColor = forumLikeColor(),
    label = label,
    stateLabel = stateLabel,
    onToggle = onToggle,
    enabled = enabled,
)

@Composable
internal fun ForumDislikeToggle(
    disliked: Boolean,
    count: Long,
    label: String,
    stateLabel: String,
    onToggle: () -> Unit,
    enabled: Boolean = true,
) = ForumReactionToggle(
    active = disliked,
    count = count,
    activeIcon = Icons.Filled.ThumbDown,
    inactiveIcon = Icons.Outlined.ThumbDown,
    activeColor = MaterialTheme.colorScheme.onSurface,
    label = label,
    stateLabel = stateLabel,
    onToggle = onToggle,
    enabled = enabled,
)

/**
 * Row with the avatar column on the left. With [connectBelow] a line runs from
 * under the avatar to the bottom of the row, joining it to the next reply of
 * the same conversation, like the reply chains on X.
 */
@Composable
internal fun ForumThreadRow(
    avatar: @Composable () -> Unit,
    connectBelow: Boolean,
    modifier: Modifier = Modifier,
    avatarSize: Dp = 40.dp,
    connectAbove: Boolean = false,
    content: @Composable () -> Unit,
) {
    val line = MaterialTheme.colorScheme.outlineVariant
    Row(
        modifier = modifier.height(IntrinsicSize.Min),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Column(
            modifier = Modifier
                .width(avatarSize)
                .fillMaxHeight(),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Box(
                Modifier
                    .width(2.dp)
                    .height(if (connectAbove) 8.dp else 0.dp)
                    .background(line),
            )
            avatar()
            if (connectBelow) {
                Box(
                    Modifier
                        .padding(top = 4.dp)
                        .width(2.dp)
                        .weight(1f)
                        .background(line, CircleShape),
                )
            }
        }
        Box(Modifier.weight(1f)) { content() }
    }
}

/**
 * Floating pill for replies that arrived while reading. Tapping it brings
 * them into the list and scrolls to the top, instead of moving the list under
 * the reader's finger.
 */
@Composable
internal fun ForumNewRepliesPill(
    visible: Boolean,
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnimatedVisibility(
        visible = visible,
        enter = slideInVertically { -it } + fadeIn(),
        exit = slideOutVertically { -it } + fadeOut(),
        modifier = modifier,
    ) {
        Surface(
            onClick = onClick,
            shape = CircleShape,
            color = MaterialTheme.colorScheme.primary,
            contentColor = MaterialTheme.colorScheme.onPrimary,
            shadowElevation = 6.dp,
        ) {
            Row(
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                Icon(Icons.Filled.ArrowUpward, contentDescription = null, modifier = Modifier.size(16.dp))
                Text(label, style = MaterialTheme.typography.labelLarge, fontWeight = FontWeight.Bold)
            }
        }
    }
}
