package com.anpfuel.app.ui.contributors

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.PersonOutline
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.app.ui.theme.PremiumColors

const val CONTRIBUTORS_ENTRY_TEST_TAG = "profile_contributors_entry"

/** Last Profile item: highlighted project-contributors entry, Premium accent. */
@Composable
fun ProfileContributorsEntry(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = PremiumColors.current()
    Card(
        onClick = onClick,
        modifier = modifier
            .fillMaxWidth()
            .testTag(CONTRIBUTORS_ENTRY_TEST_TAG),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(
            containerColor = palette.container,
            contentColor = palette.content,
        ),
        border = BorderStroke(1.dp, palette.border),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(
                imageVector = Icons.Default.PersonOutline,
                contentDescription = null,
                tint = palette.accent,
                modifier = Modifier.size(24.dp),
            )
            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(2.dp),
            ) {
                Text(
                    text = stringResource(R.string.contributors_entry_title),
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold,
                )
                Text(
                    text = stringResource(R.string.contributors_entry_subtitle),
                    style = MaterialTheme.typography.bodySmall,
                    color = palette.accent,
                )
            }
            Icon(
                imageVector = Icons.Default.ChevronRight,
                contentDescription = null,
                tint = palette.accent,
            )
        }
    }
}
