package com.anpfuel.app.ui.map

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.maps.engine.MapOverlayList
import com.anpfuel.maps.engine.MapOverlayListItem

/** Shown only after a submitted search or explicit native cluster selection. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun MapStationPicker(
    title: String,
    items: List<MapOverlayListItem>,
    onSelect: (String) -> Unit,
    onDismiss: () -> Unit,
    tag: String,
) {
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = MaterialTheme.colorScheme.surface,
        modifier = Modifier.testTag(tag),
    ) {
        Column(Modifier.fillMaxWidth().fillMaxHeight(0.7f)) {
            Row(
                Modifier.fillMaxWidth().padding(start = 24.dp, end = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(title, style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f))
                IconButton(onClick = onDismiss) {
                    Icon(Icons.Default.Close, stringResource(R.string.action_dismiss))
                }
            }
            MapOverlayList(items, onSelect, Modifier.fillMaxWidth().weight(1f).testTag("map-picker-list"))
        }
    }
}
