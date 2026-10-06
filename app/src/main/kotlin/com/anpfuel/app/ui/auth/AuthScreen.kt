package com.anpfuel.app.ui.auth

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.AlertDialog
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
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
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
 * FREE-account screen: anonymous key accounts (P13 key extension).
 *
 * One tap on "Criar conta" plus a username mints the account and shows
 * the server-issued key exactly once; the key alone logs in afterwards
 * (no email, provider or device ceremony). Session tokens never render:
 * only the account key shows, on its one-display screen and the Profile
 * backup card. Provider completion arrives via the
 * `anpfuel://auth/callback` deep link; the native SDK minting real id
 * tokens is device-gated (release horizon).
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
        onUsernameChange = viewModel::onUsernameChange,
        onKeyChange = viewModel::onKeyChange,
        onCreateAccount = viewModel::onCreateAccount,
        onLoginWithKey = viewModel::onLoginWithKey,
        onGoToKeyEntry = viewModel::onGoToKeyEntry,
        onBackToUsername = viewModel::onBackToUsername,
        onProviderClick = viewModel::onProviderClick,
        onCancelProviderLink = viewModel::onCancelProviderLink,
        onLogout = viewModel::onLogout,
        onDeleteAccount = viewModel::onDeleteAccount,
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
    onUsernameChange: (String) -> Unit,
    onKeyChange: (String) -> Unit,
    onCreateAccount: () -> Unit,
    onLoginWithKey: () -> Unit,
    onGoToKeyEntry: () -> Unit,
    onBackToUsername: () -> Unit,
    onProviderClick: (String) -> Unit,
    onCancelProviderLink: () -> Unit,
    onLogout: () -> Unit,
    onDeleteAccount: () -> Unit,
    onDismissError: () -> Unit,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var showDeleteConfirm by remember { mutableStateOf(false) }
    val clipboard = androidx.compose.ui.platform.LocalClipboardManager.current
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
                AuthStep.USERNAME_ENTRY -> {
                    Text(text = stringResource(R.string.auth_key_subtitle))
                    OutlinedTextField(
                        value = state.username,
                        onValueChange = onUsernameChange,
                        label = { Text(text = stringResource(R.string.auth_username_label)) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Button(
                        onClick = onCreateAccount,
                        enabled = state.username.length >= 3,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.auth_create_account))
                    }
                    TextButton(onClick = onGoToKeyEntry) {
                        Text(text = stringResource(R.string.auth_have_key))
                    }
                }
                AuthStep.KEY_ISSUED -> {
                    Text(text = stringResource(R.string.auth_key_issued_title))
                    Text(
                        text = groupedKey(state.issuedKey),
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        text = stringResource(R.string.auth_key_issued_warning),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                    OutlinedButton(
                        onClick = {
                            clipboard.setText(androidx.compose.ui.text.AnnotatedString(state.issuedKey))
                        },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.auth_copy_key))
                    }
                    Button(
                        onClick = onLoginWithKey,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.auth_saved_enter))
                    }
                    TextButton(onClick = onBackToUsername) {
                        Text(text = stringResource(R.string.action_back))
                    }
                }
                AuthStep.KEY_ENTRY -> {
                    Text(text = stringResource(R.string.auth_key_subtitle))
                    OutlinedTextField(
                        value = state.accountKey,
                        onValueChange = onKeyChange,
                        label = { Text(text = stringResource(R.string.auth_key_label)) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Button(
                        onClick = onLoginWithKey,
                        enabled = state.accountKey.isNotBlank(),
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.auth_key_login))
                    }
                    TextButton(onClick = onBackToUsername) {
                        Text(text = stringResource(R.string.action_back))
                    }
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
                    OutlinedButton(
                        onClick = { showDeleteConfirm = true },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(text = stringResource(R.string.auth_delete_account))
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
    if (showDeleteConfirm) {
        AlertDialog(
            onDismissRequest = { showDeleteConfirm = false },
            title = { Text(text = stringResource(R.string.auth_delete_title)) },
            text = { Text(text = stringResource(R.string.auth_delete_copy)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        showDeleteConfirm = false
                        onDeleteAccount()
                    },
                ) {
                    Text(text = stringResource(R.string.auth_delete_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = { showDeleteConfirm = false }) {
                    Text(text = stringResource(R.string.action_cancel))
                }
            },
        )
    }
}

@Composable
private fun errorText(error: AuthUiError): String = stringResource(
    when (error) {
        AuthUiError.InvalidInput -> R.string.auth_error_input
        AuthUiError.WrongCode -> R.string.auth_error_code
        AuthUiError.WrongKey -> R.string.auth_error_key
        AuthUiError.UsernameTaken -> R.string.auth_error_username_taken
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

/** Groups a raw account key in fours for transcription. */
private fun groupedKey(raw: String): String =
    raw.chunked(4).joinToString(" ")

@Preview(showBackground = true)
@Composable
private fun AuthScreenEmailPreview() {
    AnpFuelTheme {
        AuthScreen(
            state = AuthUiState(step = AuthStep.USERNAME_ENTRY, username = "ana123"),
            onEmailChange = {},
            onCodeChange = {},
            onRequestCode = {},
            onConsumeCode = {},
            onBackToEmail = {},
            onUsernameChange = {},
            onKeyChange = {},
            onCreateAccount = {},
            onLoginWithKey = {},
            onGoToKeyEntry = {},
            onBackToUsername = {},
            onProviderClick = {},
            onCancelProviderLink = {},
            onLogout = {},
            onDeleteAccount = {},
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
            onUsernameChange = {},
            onKeyChange = {},
            onCreateAccount = {},
            onLoginWithKey = {},
            onGoToKeyEntry = {},
            onBackToUsername = {},
            onProviderClick = {},
            onCancelProviderLink = {},
            onLogout = {},
            onDeleteAccount = {},
            onDismissError = {},
            onNavigateBack = {},
        )
    }
}
