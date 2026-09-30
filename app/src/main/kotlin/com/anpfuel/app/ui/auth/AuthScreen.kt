package com.anpfuel.app.ui.auth

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.theme.AnpFuelTheme

/**
 * FREE-account login screen (P13-T05C).
 *
 * Email access-code first (the only backend signup path), Google/Apple
 * link buttons once authenticated (the backend links verified subjects
 * onto the email-created account; equal emails never merge). Tokens
 * never render: the authenticated state shows status only. Provider
 * completion arrives via the `anpfuel://auth/callback` deep link; the
 * native SDK minting real id tokens is device-gated (release horizon).
 */
@Composable
fun AuthRoute(
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: AuthViewModel = hiltViewModel(),
    providerArg: String = "",
    idTokenArg: String = "",
    nonceArg: String = "",
    stateArg: String = "",
) {
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    LaunchedEffect(providerArg, idTokenArg, nonceArg, stateArg) {
        if (providerArg.isNotEmpty()) {
            viewModel.onProviderCallback(providerArg, idTokenArg, nonceArg, stateArg)
        }
    }
    AuthScreen(
        state = uiState,
        onEmailChange = viewModel::onEmailChange,
        onCodeChange = viewModel::onCodeChange,
        onRequestCode = viewModel::onRequestCode,
        onConsumeCode = viewModel::onConsumeCode,
        onBackToEmail = viewModel::onBackToEmail,
        onProviderClick = viewModel::onProviderClick,
        onCancelProviderLink = viewModel::onCancelProviderLink,
        onLogout = viewModel::onLogout,
        onDismissError = viewModel::onDismissError,
        onNavigateBack = onNavigateBack,
        modifier = modifier,
    )
}

@Composable
fun AuthScreen(
    state: AuthUiState,
    onEmailChange: (String) -> Unit,
    onCodeChange: (String) -> Unit,
    onRequestCode: () -> Unit,
    onConsumeCode: () -> Unit,
    onBackToEmail: () -> Unit,
    onProviderClick: (String) -> Unit,
    onCancelProviderLink: () -> Unit,
    onLogout: () -> Unit,
    onDismissError: () -> Unit,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp, Alignment.CenterVertically),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                text = stringResource(R.string.auth_title),
                style = MaterialTheme.typography.headlineSmall,
            )
            when (state.step) {
                AuthStep.CHECKING, AuthStep.BUSY -> {
                    CircularProgressIndicator()
                }
                AuthStep.EMAIL_ENTRY, AuthStep.CODE_SENT -> {
                    OutlinedTextField(
                        value = state.email,
                        onValueChange = onEmailChange,
                        label = { Text(text = stringResource(R.string.auth_email_label)) },
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
                        singleLine = true,
                        enabled = state.step == AuthStep.EMAIL_ENTRY,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    if (state.step == AuthStep.CODE_SENT) {
                        OutlinedTextField(
                            value = state.code,
                            onValueChange = onCodeChange,
                            label = { Text(text = stringResource(R.string.auth_code_label)) },
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                            singleLine = true,
                            modifier = Modifier.fillMaxWidth(),
                        )
                        Button(
                            onClick = onConsumeCode,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(text = stringResource(R.string.auth_confirm))
                        }
                        TextButton(onClick = onBackToEmail) {
                            Text(text = stringResource(R.string.action_back))
                        }
                    } else {
                        Button(
                            onClick = onRequestCode,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(text = stringResource(R.string.auth_continue))
                        }
                    }
                }
                AuthStep.PROVIDER_PENDING -> {
                    CircularProgressIndicator()
                    Text(text = stringResource(R.string.auth_pending_provider, state.pendingProvider))
                    TextButton(onClick = onCancelProviderLink) {
                        Text(text = stringResource(R.string.action_cancel))
                    }
                }
                AuthStep.AUTHENTICATED -> {
                    Text(text = stringResource(R.string.auth_logged_in))
                    OutlinedButton(
                        onClick = { onProviderClick("google") },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.auth_google))
                    }
                    OutlinedButton(
                        onClick = { onProviderClick("apple") },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.auth_apple))
                    }
                    Button(
                        onClick = onLogout,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.auth_logout))
                    }
                }
            }
            state.error?.let { error ->
                TextButton(onClick = onDismissError) {
                    Text(
                        text = errorText(error),
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
            Spacer(modifier = Modifier.height(8.dp))
            TextButton(onClick = onNavigateBack) {
                Text(text = stringResource(R.string.action_back))
            }
        }
    }
}

@Composable
private fun errorText(error: AuthUiError): String = stringResource(
    when (error) {
        AuthUiError.InvalidInput -> R.string.auth_error_input
        AuthUiError.WrongCode -> R.string.auth_error_code
        AuthUiError.ExpiredCode -> R.string.auth_error_expired
        AuthUiError.LockedCode -> R.string.auth_error_locked
        AuthUiError.SessionExpired -> R.string.auth_error_session
        AuthUiError.Suspended -> R.string.auth_error_suspended
        AuthUiError.Deleted -> R.string.auth_error_deleted
        AuthUiError.Offline -> R.string.auth_error_offline
        AuthUiError.ProviderDenied -> R.string.auth_error_provider
        AuthUiError.LoginFirst -> R.string.auth_error_login_first
        AuthUiError.Unknown -> R.string.auth_error_unknown
    },
)

@Preview(showBackground = true)
@Composable
private fun AuthScreenEmailPreview() {
    AnpFuelTheme {
        AuthScreen(
            state = AuthUiState(step = AuthStep.EMAIL_ENTRY, email = "case-01@example.invalid"),
            onEmailChange = {},
            onCodeChange = {},
            onRequestCode = {},
            onConsumeCode = {},
            onBackToEmail = {},
            onProviderClick = {},
            onCancelProviderLink = {},
            onLogout = {},
            onDismissError = {},
            onNavigateBack = {},
        )
    }
}

@Preview(showBackground = true)
@Composable
private fun AuthScreenAuthenticatedPreview() {
    AnpFuelTheme {
        AuthScreen(
            state = AuthUiState(step = AuthStep.AUTHENTICATED),
            onEmailChange = {},
            onCodeChange = {},
            onRequestCode = {},
            onConsumeCode = {},
            onBackToEmail = {},
            onProviderClick = {},
            onCancelProviderLink = {},
            onLogout = {},
            onDismissError = {},
            onNavigateBack = {},
        )
    }
}
