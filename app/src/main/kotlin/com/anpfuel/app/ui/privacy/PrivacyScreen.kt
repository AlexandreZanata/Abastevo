package com.anpfuel.app.ui.privacy

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.settings.TrainingConsentSettings
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar

/**
 * Perfil > Privacidade (B-BR-CS06 / BUC-CS05).
 *
 * Privacy guarantees (no public precise GPS, 24h photo expiry across
 * all copies, free account independent of the training choice) plus
 * the shared optional training-consent settings (default OFF,
 * server authoritative, explicit terms review). Reached from the
 * Perfil tools list below Help; same component instance shape as
 * Configurações uses, no forked consent logic.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PrivacyScreen(
    onNavigateBack: () -> Unit,
    onNavigateToAuth: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(text = stringResource(R.string.privacy_screen_title)) },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            ) {
                Column(
                    modifier = Modifier.padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        Icon(
                            imageVector = Icons.Default.Lock,
                            contentDescription = null,
                            tint = MaterialTheme.colorScheme.primary,
                        )
                        Text(
                            text = stringResource(R.string.profile_privacy_section),
                            style = MaterialTheme.typography.titleSmall,
                            modifier = Modifier.semantics { heading() },
                        )
                    }
                    Text(
                        text = stringResource(R.string.profile_privacy_body),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }

            TrainingConsentSettings(onSignIn = onNavigateToAuth)
        }
    }
}
