package com.anpfuel.app.ui.accessibility

import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics

fun Modifier.headingSemantics(): Modifier =
    semantics { heading() }

fun Modifier.liveRegionSemantics(): Modifier =
    semantics { liveRegion = LiveRegionMode.Polite }

fun Modifier.accessibilityDescription(description: String): Modifier =
    semantics { contentDescription = description }
