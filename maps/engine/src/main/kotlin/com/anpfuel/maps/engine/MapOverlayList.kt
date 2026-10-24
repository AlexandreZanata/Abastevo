package com.anpfuel.maps.engine

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import app.organicmaps.sdk.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MapOverlayList(
    items: List<MapOverlayListItem>,
    onSelect: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    LazyColumn(modifier = modifier) {
        items(items, key = { it.uuid }) { item ->
            ListItem(
                leadingContent = {
                    Row(horizontalArrangement = Arrangement.spacedBy(2.dp)) {
                        if (item.type != MapStationType.CHARGING) {
                            Icon(
                                painter = painterResource(R.drawable.ic_map_station_combustion),
                                contentDescription = null,
                                modifier = Modifier.size(24.dp).testTag("station-icon-combustion"),
                            )
                        }
                        if (item.type != MapStationType.FUEL) {
                            Icon(
                                painter = painterResource(R.drawable.ic_map_station_electric),
                                contentDescription = null,
                                modifier = Modifier.size(24.dp).testTag("station-icon-electric"),
                            )
                        }
                    }
                },
                headlineContent = { Text(item.title) },
                supportingContent = { Text("${item.typeLabel} - ${item.locationLabel}") },
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 48.dp)
                    .clickable { onSelect(item.uuid) }
                    .semantics(mergeDescendants = true) {
                        contentDescription = item.contentDescription
                    },
            )
        }
    }
}
