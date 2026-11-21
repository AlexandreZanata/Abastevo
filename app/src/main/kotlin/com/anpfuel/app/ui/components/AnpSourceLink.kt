package com.anpfuel.app.ui.components

import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import com.anpfuel.app.R

@Composable
fun AnpSourceLink(modifier: Modifier = Modifier) {
    val source = stringResource(R.string.anp_official_url)
    val uriHandler = LocalUriHandler.current
    TextButton(
        onClick = { uriHandler.openUri(source) },
        modifier = modifier.testTag("anp_source_link"),
    ) {
        Text(stringResource(R.string.prices_anp_attribution))
    }
}
