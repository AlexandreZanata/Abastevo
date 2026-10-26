package com.anpfuel.app.ui.map

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import com.anpfuel.app.ui.components.SkeletonButton
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.app.ui.components.SkeletonLine

/**
 * Map-specific first-frame skeleton. It mirrors the final overlay geometry
 * (top controls and bottom action panel) so the map surface never starts at a
 * different size and then shrinks when filters or period chips arrive.
 */
@Composable
fun MapScreenSkeleton(modifier: Modifier = Modifier) {
    val surface = MaterialTheme.colorScheme.surfaceContainerLow
    val road = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.55f)
    val park = MaterialTheme.colorScheme.surfaceContainerHigh.copy(alpha = 0.72f)
    Box(
        modifier
            .fillMaxSize()
            .background(surface)
            .testTag("map-skeleton"),
    ) {
        Canvas(Modifier.fillMaxSize()) {
            val w = size.width
            val h = size.height
            drawRoundRect(
                color = park,
                topLeft = Offset(w * 0.58f, h * 0.18f),
                size = androidx.compose.ui.geometry.Size(w * 0.28f, h * 0.20f),
                cornerRadius = androidx.compose.ui.geometry.CornerRadius(28f, 28f),
            )
            drawRoundRect(
                color = park,
                topLeft = Offset(w * 0.12f, h * 0.62f),
                size = androidx.compose.ui.geometry.Size(w * 0.22f, h * 0.18f),
                cornerRadius = androidx.compose.ui.geometry.CornerRadius(24f, 24f),
            )
            drawLine(road, Offset(0f, h * 0.48f), Offset(w, h * 0.37f), strokeWidth = 18f)
            drawLine(road, Offset(w * 0.18f, 0f), Offset(w * 0.46f, h), strokeWidth = 12f)
            drawLine(road, Offset(w * 0.72f, 0f), Offset(w * 0.54f, h), strokeWidth = 10f)
            drawLine(road, Offset(0f, h * 0.82f), Offset(w, h * 0.74f), strokeWidth = 9f)
        }

        Column(
            Modifier
                .fillMaxSize()
                .statusBarsPadding()
                .padding(horizontal = 12.dp, vertical = 8.dp),
        ) {
            Surface(
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(20.dp),
                color = MaterialTheme.colorScheme.surface.copy(alpha = 0.96f),
                tonalElevation = 2.dp,
            ) {
                SkeletonGroup(Modifier.padding(12.dp)) {
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        SkeletonLine(modifier = Modifier.weight(1f), height = 24.dp)
                        SkeletonLine(modifier = Modifier.width(72.dp), height = 24.dp)
                    }
                    SkeletonLine(height = 56.dp)
                    SkeletonLine(width = 132.dp, height = 12.dp)
                }
            }
            Spacer(Modifier.weight(1f))
            Surface(
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(20.dp),
                color = MaterialTheme.colorScheme.surface.copy(alpha = 0.96f),
                tonalElevation = 2.dp,
            ) {
                SkeletonGroup(Modifier.padding(12.dp)) {
                    SkeletonLine(width = 132.dp, height = 20.dp)
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        SkeletonButton(Modifier.weight(1f))
                        SkeletonButton(Modifier.weight(1f))
                        SkeletonButton(Modifier.weight(1f))
                    }
                }
            }
        }
    }
}
