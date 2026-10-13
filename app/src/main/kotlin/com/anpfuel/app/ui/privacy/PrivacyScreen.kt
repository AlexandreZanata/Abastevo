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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.settings.TrainingConsentPanel
import com.anpfuel.app.settings.TrainingConsentUiState
import com.anpfuel.app.settings.TrainingConsentViewModel
import com.anpfuel.app.ui.auth.AuthStep
import com.anpfuel.app.ui.auth.AuthViewModel
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar

/**
 * Perfil > Privacidade (B-BR-CS06 / BUC-CS05).
 *
 * Privacy guarantees (no public precise GPS, 24h photo expiry across
 * all copies, free account independent of training choice) plus the
 * optional training-consent switch (default OFF, server authoritative).
 * Guest sees guarantees only; the consent panel renders nothing for
 * guests and the host never invents a grant. A terms mismatch refuses
 * server-side with an explicit rejection.
 */
private const val CURRENT_TERMS_VERSION = "training-terms-v1"

/**
 * Provisional digest bound to [CURRENT_TERMS_VERSION] until the
 * verified terms land (mirrors backend default). Server authoritative:
 * a rotated digest refuses with TERMS_OUTDATED, never a false grant.
 */
private const val CURRENT_TERMS_DIGEST =
    "741dcce562dd29959c5fca09ad5ebd6f986fa6e94c80c2e556fe3f6f80beac48"

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PrivacyScreen(
    onNavigateBack: () -> Unit,
    onNavigateToAuth: () -> Unit,
    modifier: Modifier = Modifier,
    authViewModel: AuthViewModel = hiltViewModel(),
    consentViewModel: TrainingConsentViewModel = hiltViewModel(),
) {
    val authState by authViewModel.uiState.collectAsStateWithLifecycle()
    val signedIn = authState.step in listOf(AuthStep.AUTHENTICATED, AuthStep.OFFLINE_ACCOUNT)
    androidx.lifecycle.compose.LifecycleEventEffect(androidx.lifecycle.Lifecycle.Event.ON_RESUME) {
        authViewModel.refreshAccount()
    }
    LaunchedEffect(signedIn) {
        consentViewModel.prepare(if (signedIn) "signed-in" else "")
    }
    val consentState by consentViewModel.state.collectAsStateWithLifecycle()

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

            TrainingConsentPanel(
                state = consentState,
                onToggle = { enabled ->
                    val content = consentState as? TrainingConsentUiState.Content
                    if (enabled) {
                        consentViewModel.setEnabled(
                            true,
                            content?.termsVersion ?: CURRENT_TERMS_VERSION,
                            content?.termsDigest ?: CURRENT_TERMS_DIGEST,
                        )
                    } else {
                        consentViewModel.setEnabled(
                            false,
                            CURRENT_TERMS_VERSION,
                            CURRENT_TERMS_DIGEST,
                        )
                    }
                },
                onRetry = { consentViewModel.retry() },
                onSignIn = onNavigateToAuth,
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}
