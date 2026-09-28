package com.anpfuel.app.ui.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.accessibility.headingSemantics
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.theme.AnpFuelTheme

@Composable
fun LocationPromptStep(
    isResolvingLocation: Boolean,
    onUseDeviceLocation: () -> Unit,
    onChooseManualLocation: () -> Unit,
    modifier: Modifier = Modifier,
) {
    if (isResolvingLocation) {
        LoadingState(
            message = stringResource(R.string.onboarding_location_resolving),
            modifier = modifier.fillMaxSize(),
        )
        return
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp, vertical = 16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp, Alignment.CenterVertically),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = stringResource(R.string.onboarding_location_prompt_title),
            style = MaterialTheme.typography.headlineMedium,
            color = MaterialTheme.colorScheme.onBackground,
            textAlign = TextAlign.Center,
            modifier = Modifier.headingSemantics(),
        )
        Text(
            text = stringResource(R.string.onboarding_location_prompt_body),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
        Button(
            onClick = onUseDeviceLocation,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(text = stringResource(R.string.onboarding_location_use_device))
        }
        TextButton(
            onClick = onChooseManualLocation,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(text = stringResource(R.string.onboarding_location_choose_manual))
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun LocationPromptStepPreview() {
    AnpFuelTheme {
        LocationPromptStep(
            isResolvingLocation = false,
            onUseDeviceLocation = {},
            onChooseManualLocation = {},
        )
    }
}
