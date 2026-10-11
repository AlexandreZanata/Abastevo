package com.anpfuel.app.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Checkbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R

/**
 * CS11C — Per-photo training opt-in row (B-BR-CS06).
 *
 * Explicit per-photo permission at review: unchecked by default,
 * checkable only while server consent is granted. Without granted
 * consent the box stays disabled with the gate explanation and
 * [onChange] never fires a check. State hoists to the caller (the
 * review screen owns [TrainingPhotoOptInState]); this row renders
 * one photo decision with its TalkBack description and no fixed
 * heights.
 */
@Composable
fun TrainingPhotoOptInRow(
    photoLabel: String,
    allowed: Boolean,
    consentGranted: Boolean,
    onChange: (allowed: Boolean) -> Unit,
    modifier: Modifier = Modifier,
) {
    val description = trainingPhotoOptInTalkBackDescription(photoLabel, allowed, consentGranted)
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Checkbox(
            checked = allowed && consentGranted,
            onCheckedChange = if (consentGranted) onChange else null,
            enabled = consentGranted,
            modifier = Modifier.semantics { contentDescription = description },
        )
        Text(
            text = if (consentGranted) {
                stringResource(R.string.training_photo_opt_in)
            } else {
                stringResource(R.string.training_photo_opt_in_locked)
            },
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * CS11C TalkBack description: the photo label plus the opt-in
 * outcome and the consent gate, never a bare checkbox alone. Pure
 * for unit tests.
 */
internal fun trainingPhotoOptInTalkBackDescription(
    photoLabel: String,
    allowed: Boolean,
    consentGranted: Boolean,
): String {
    val label = photoLabel.trim().takeIf { it.isNotBlank() } ?: "Photo"
    if (allowed && consentGranted) {
        return "$label, allowed for training review, consent granted"
    }
    if (allowed) {
        return "$label, not allowed, needs consent first"
    }
    return "$label, not allowed"
}
