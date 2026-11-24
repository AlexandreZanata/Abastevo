package com.anpfuel.app.ui.auth

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.BuildConfig
import com.anpfuel.app.R
import com.anpfuel.app.ui.account.AccountStatusCard
import com.anpfuel.app.ui.account.SignedInAccountContent
import com.anpfuel.app.ui.components.AbastevoBrandBanner
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.OutlinedButton
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.domain.portable.PortableAuth

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AuthRoute(
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: AuthViewModel = hiltViewModel(),
    requireFreshProof: Boolean = false,
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    LaunchedEffect(viewModel, requireFreshProof) {
        if (requireFreshProof) viewModel.beginReauthentication()
    }
    LaunchedEffect(viewModel) {
        viewModel.navigation.collect { onNavigateBack() }
    }
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) {
        if (!requireFreshProof) viewModel.refreshAccount()
    }
    LaunchedEffect(state.step, state.profile) {
        if (!requireFreshProof) viewModel.resumePublicNameConfirmation()
    }
    val step = state.formStep
    val canReturnToEntry = !requireFreshProof && step in setOf(
        AuthStep.SIGNUP_FORM, AuthStep.RECOVERY_FORM, AuthStep.KEY_ENTRY, AuthStep.CODE_SENT,
    ) && state.purpose != PortableAuth.CodePurpose.ATTACH
    val canCancelEdit = !requireFreshProof && state.editingProfile && step != AuthStep.CONFIRM_NAME
    val onBack = when {
        canReturnToEntry -> viewModel::onBackToEntry
        canCancelEdit -> viewModel::onCancelProfileEdit
        else -> onNavigateBack
    }
    BackHandler(enabled = canReturnToEntry || canCancelEdit, onBack = onBack)
    SecureAccountWindow()
    var showDeleteConfirm by remember { mutableStateOf(false) }
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            AnpTopAppBar(title = { Text(stringResource(R.string.auth_title)) }, onNavigateUp = onBack)
        },
    ) { padding ->
        Column(
            modifier = Modifier.fillMaxSize().padding(padding).imePadding()
                .verticalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            when (step) {
                AuthStep.CHECKING -> SkeletonCard(height = 180.dp)
                AuthStep.KEY_ISSUED -> {
                    val recovered = state.purpose == PortableAuth.CodePurpose.RECOVERY
                    AccountStatusCard(
                        title = stringResource(if (recovered) R.string.auth_new_key_title else R.string.auth_key_issued_title),
                        subtitle = stringResource(
                            when {
                                recovered -> R.string.auth_new_key_copy
                                state.issuedKeyCanBeRecovered -> R.string.auth_key_issued_warning
                                else -> R.string.auth_legacy_key_issued_warning
                            },
                        ),
                    )
                    AccountKeyCard(AuthFlow.KeyBackup(state.issuedUsername, state.issuedKey))
                    Button(onClick = viewModel::onLoginWithKey, modifier = Modifier.fillMaxWidth()) {
                        Text(stringResource(R.string.auth_saved_enter))
                    }
                    AuthError(state, viewModel::onDismissError)
                }
                AuthStep.AUTHENTICATED, AuthStep.OFFLINE_ACCOUNT -> {
                    SignedInAccountContent(state, viewModel, onDelete = { showDeleteConfirm = true })
                    Button(onClick = onNavigateBack, modifier = Modifier.fillMaxWidth()) {
                        Text(stringResource(R.string.auth_continue))
                    }
                }
                else -> {
                    AbastevoBrandBanner()
                    AccountAuthForm(state, viewModel, onBack = onBack)
                }
            }
        }
    }
    if (showDeleteConfirm) {
        ConfirmAccountDeletion(
            onDismiss = { showDeleteConfirm = false },
            onConfirm = {
                showDeleteConfirm = false
                viewModel.onDeleteAccount()
            },
        )
    }
}

internal val AuthUiState.formStep: AuthStep
    get() = if (step == AuthStep.BUSY) busyOrigin else step

internal val AuthUiState.editingProfile: Boolean
    get() = formStep in setOf(
        AuthStep.CONFIRM_NAME, AuthStep.EDIT_NAME, AuthStep.ATTACH_EMAIL,
        AuthStep.REAUTHENTICATION, AuthStep.LINK_GOOGLE,
    ) || (formStep == AuthStep.CODE_SENT && purpose == PortableAuth.CodePurpose.ATTACH)

@Composable
internal fun AccountAuthForm(state: AuthUiState, viewModel: AuthViewModel, onBack: () -> Unit) {
    val context = LocalContext.current
    val gateway = remember(context) { CredentialManagerGoogleSignIn(context) }
    val locale = LocalConfiguration.current.locales[0].toLanguageTag()
    val busy = state.step == AuthStep.BUSY
    val step = state.formStep
    val googleEnabled = BuildConfig.GOOGLE_SERVER_CLIENT_ID.isNotBlank()
    val fill = MaterialTheme.colorScheme.surfaceContainerLow
    Card(
        modifier = Modifier.widthIn(max = 520.dp).fillMaxWidth(),
        shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = fill),
        border = AnpContainerDefaults.borderStroke(fill),
    ) {
        Column(Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            when (step) {
                AuthStep.ENTRY -> {
                    AuthHeading(R.string.auth_signup_heading)
                    if (googleEnabled) {
                        GoogleSignInButton(!busy) { viewModel.onGoogle(gateway) }
                    }
                    OutlinedButton(viewModel::onGoToKeyEntry, Modifier.fillMaxWidth(), enabled = !busy) {
                        Text(stringResource(R.string.auth_key_login))
                    }
                    Button(viewModel::beginSignUp, Modifier.fillMaxWidth(), enabled = !busy) {
                        Text(stringResource(R.string.auth_create_account))
                    }
                    TextButton(viewModel::beginRecovery, Modifier.align(Alignment.CenterHorizontally), enabled = !busy) {
                        Text(stringResource(R.string.auth_lost_key))
                    }
                }
                AuthStep.SIGNUP_FORM, AuthStep.RECOVERY_FORM, AuthStep.ATTACH_EMAIL -> {
                    val signup = step == AuthStep.SIGNUP_FORM
                    AuthHeading(
                        when (step) {
                            AuthStep.SIGNUP_FORM -> R.string.auth_create_account
                            AuthStep.ATTACH_EMAIL -> R.string.auth_recovery_email_title
                            else -> R.string.auth_lost_key
                        },
                    )
                    if (signup) PublicNameField(state.name, viewModel::onNameChange, !busy)
                    OutlinedTextField(
                        value = state.email,
                        onValueChange = viewModel::onEmailChange,
                        label = { Text(stringResource(R.string.auth_email_label)) },
                        supportingText = if (signup || step == AuthStep.ATTACH_EMAIL) {
                            { Text(stringResource(R.string.auth_email_helper)) }
                        } else null,
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
                        enabled = !busy,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Button(
                        onClick = { viewModel.onRequestCode(locale) },
                        modifier = Modifier.fillMaxWidth(),
                        enabled = !busy && state.email.isNotBlank() && (!signup || state.name.isNotBlank()),
                    ) { Text(stringResource(R.string.auth_send_code)) }
                }
                AuthStep.CODE_SENT -> {
                    AuthHeading(R.string.auth_email_verification_title)
                    Text(stringResource(R.string.auth_email_check_inbox))
                    OutlinedTextField(
                        value = state.code,
                        onValueChange = viewModel::onCodeChange,
                        label = { Text(stringResource(R.string.auth_code_label)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        enabled = !busy,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Button(
                        onClick = viewModel::onConsumeCode,
                        modifier = Modifier.fillMaxWidth(),
                        enabled = !busy && state.code.length == 6,
                    ) { Text(stringResource(R.string.auth_verify_code)) }
                    TextButton(
                        onClick = { viewModel.onRequestCode(locale) },
                        enabled = !busy && state.resendSeconds == 0,
                    ) {
                        Text(
                            if (state.resendSeconds == 0) stringResource(R.string.auth_resend_code)
                            else stringResource(R.string.auth_resend_after, state.resendSeconds),
                        )
                    }
                    TextButton(onClick = viewModel::onChangeCodeEmail, enabled = !busy) {
                        Text(stringResource(R.string.auth_change_email))
                    }
                }
                AuthStep.KEY_ENTRY, AuthStep.REAUTHENTICATION -> {
                    val reauth = step == AuthStep.REAUTHENTICATION
                    AuthHeading(if (reauth) R.string.auth_reauthenticate_title else R.string.auth_key_login)
                    if (!reauth || state.profile?.hasAccountKey != false) {
                        OutlinedTextField(
                            value = state.accountKey,
                            onValueChange = viewModel::onKeyChange,
                            label = { Text(stringResource(R.string.auth_key_label)) },
                            singleLine = true,
                            visualTransformation = PasswordVisualTransformation(),
                            keyboardOptions = KeyboardOptions(
                                keyboardType = KeyboardType.Password,
                                autoCorrectEnabled = false,
                                imeAction = ImeAction.Done,
                            ),
                            keyboardActions = KeyboardActions(onDone = {
                                if (!busy && state.accountKey.isNotBlank()) viewModel.onLoginWithKey()
                            }),
                            enabled = !busy,
                            modifier = Modifier.fillMaxWidth().testTag("account-key-entry"),
                        )
                        Button(
                            onClick = viewModel::onLoginWithKey,
                            modifier = Modifier.fillMaxWidth().testTag("account-key-submit"),
                            enabled = !busy && state.accountKey.isNotBlank(),
                        ) { Text(stringResource(R.string.auth_key_login)) }
                    }
                    if (reauth && googleEnabled && state.canReauthenticateWithGoogle) {
                        GoogleSignInButton(!busy) { viewModel.onGoogle(gateway) }
                    }
                }
                AuthStep.CONFIRM_NAME, AuthStep.EDIT_NAME -> {
                    AuthHeading(if (step == AuthStep.CONFIRM_NAME) R.string.auth_confirm_public_name else R.string.auth_edit_name)
                    PublicNameField(state.name, viewModel::onNameChange, !busy)
                    Button(
                        onClick = viewModel::onSaveName,
                        modifier = Modifier.fillMaxWidth(),
                        enabled = !busy && state.name.isNotBlank(),
                    ) { Text(stringResource(R.string.auth_save_name)) }
                }
                AuthStep.LINK_GOOGLE -> {
                    AuthHeading(R.string.auth_google)
                    if (googleEnabled) GoogleSignInButton(!busy) { viewModel.onGoogle(gateway) }
                }
                else -> Unit
            }
            if (state.error == AuthUiError.ProviderEmailInUse && step == AuthStep.ENTRY) {
                Text(
                    text = stringResource(R.string.auth_error_provider_email_in_use),
                    color = MaterialTheme.colorScheme.error,
                )
            } else {
                AuthError(state, viewModel::onDismissError)
            }
            if (busy) LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
            if (!busy && step != AuthStep.ENTRY && step != AuthStep.CONFIRM_NAME) {
                TextButton(onClick = onBack) { Text(stringResource(R.string.action_back)) }
            }
        }
    }
}

@Composable
private fun AuthHeading(resource: Int) {
    Text(
        stringResource(resource),
        style = MaterialTheme.typography.headlineSmall,
        modifier = Modifier.semantics { heading() },
    )
}

@Composable
private fun PublicNameField(name: String, onChange: (String) -> Unit, enabled: Boolean) {
    OutlinedTextField(
        value = name,
        onValueChange = onChange,
        label = { Text(stringResource(R.string.auth_public_name)) },
        supportingText = { Text(stringResource(R.string.auth_name_helper)) },
        singleLine = true,
        enabled = enabled,
        modifier = Modifier.fillMaxWidth(),
    )
}

@Composable
private fun GoogleSignInButton(enabled: Boolean, onClick: () -> Unit) {
    Button(
        onClick = onClick,
        modifier = Modifier.fillMaxWidth(),
        enabled = enabled,
        colors = ButtonDefaults.buttonColors(containerColor = Color.White, contentColor = Color(0xFF1F1F1F)),
        border = BorderStroke(1.dp, Color(0xFF747775)),
        elevation = null,
        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 10.dp),
    ) {
        Image(
            painterResource(R.drawable.google_sign_in_g),
            contentDescription = null,
            modifier = Modifier.size(20.dp),
        )
        Text(
            text = stringResource(R.string.auth_continue_google),
            fontFamily = FontFamily(
                Font(
                    R.font.google_sans,
                    weight = FontWeight.Medium,
                ),
            ),
            fontWeight = FontWeight.Medium,
            fontSize = 14.sp,
            lineHeight = 20.sp,
            modifier = Modifier.padding(start = 10.dp),
        )
    }
}

@Composable
internal fun AuthError(state: AuthUiState, onDismiss: () -> Unit) {
    state.error?.let { error ->
        val resource = when (error) {
            AuthUiError.InvalidInput -> R.string.auth_error_input
            AuthUiError.WrongCode -> R.string.auth_error_code
            AuthUiError.WrongKey -> R.string.auth_error_key
            AuthUiError.ExpiredCode -> R.string.auth_error_expired
            AuthUiError.LockedCode -> R.string.auth_error_locked
            AuthUiError.SessionExpired -> R.string.auth_error_session
            AuthUiError.Suspended -> R.string.auth_error_suspended
            AuthUiError.Deleted -> R.string.auth_error_deleted
            AuthUiError.Offline -> R.string.auth_error_offline
            AuthUiError.ProviderDenied -> R.string.auth_error_provider
            AuthUiError.SecureStorage -> R.string.auth_error_secure_storage
            AuthUiError.Unknown -> R.string.auth_error_unknown
            AuthUiError.InvalidName -> R.string.auth_error_public_name
            AuthUiError.AddressLinked -> R.string.auth_error_address_linked
            AuthUiError.RecoveryNoAccount -> R.string.auth_error_recovery_no_account
            AuthUiError.ProviderEmailInUse -> R.string.auth_error_provider_email_in_use
            AuthUiError.ReauthenticationRequired -> R.string.auth_error_fresh_login
            AuthUiError.EmailLoginRetired -> R.string.auth_error_email_login_retired
            AuthUiError.GoogleUnavailable -> R.string.auth_error_google_unavailable
        }
        TextButton(onClick = onDismiss) {
            Text(stringResource(resource), color = MaterialTheme.colorScheme.error)
        }
    }
}

@Composable
internal fun ConfirmAccountDeletion(onDismiss: () -> Unit, onConfirm: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.auth_delete_title)) },
        text = { Text(stringResource(R.string.auth_delete_copy)) },
        confirmButton = { TextButton(onConfirm) { Text(stringResource(R.string.auth_delete_confirm)) } },
        dismissButton = { TextButton(onDismiss) { Text(stringResource(R.string.action_back)) } },
    )
}
