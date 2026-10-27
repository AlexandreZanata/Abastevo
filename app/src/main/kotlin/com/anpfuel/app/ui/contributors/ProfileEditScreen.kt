package com.anpfuel.app.ui.contributors

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.components.SkeletonCard
import com.anpfuel.app.ui.components.SkeletonGroup
import com.anpfuel.domain.contributor.ProfileEdit
import com.anpfuel.domain.contributor.ProfileLinkInput
import com.anpfuel.domain.contributor.ProfileVisibility
import com.anpfuel.domain.contributor.profileEditError

/**
 * B-BR-CTB profile self-service (CTB-08/12), signed-in only.
 *
 * Client validation mirrors the server bounds; the server stays
 * authoritative and its codes surface through the notice row.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProfileEditScreen(
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ProfileViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    LaunchedEffect(viewModel) { viewModel.refresh() }
    ProfileContent(
        state = state,
        onSave = viewModel::save,
        onRetry = viewModel::refresh,
        onNavigateBack = onNavigateBack,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProfileContent(
    state: ProfileUiState,
    onSave: (ProfileEdit) -> Unit,
    onRetry: () -> Unit,
    onNavigateBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = { Text(stringResource(R.string.contributor_profile_screen_title)) },
                onNavigateUp = onNavigateBack,
            )
        },
    ) { padding ->
        when {
            state.loading -> SkeletonGroup(modifier = Modifier.fillMaxWidth().padding(padding)) {
                SkeletonCard(height = 96.dp)
                SkeletonCard(height = 64.dp)
            }

            state.notice != null -> Column(
                modifier = Modifier.fillMaxSize().padding(padding).padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text(
                    text = stringResource(
                        when (state.notice) {
                            ProfileNotice.OFFLINE -> R.string.contributors_offline
                            ProfileNotice.DENIED -> R.string.claim_signed_out
                            ProfileNotice.QUOTA -> R.string.profile_quota
                            ProfileNotice.CONFLICT -> R.string.profile_conflict
                            ProfileNotice.FORBIDDEN -> R.string.profile_forbidden
                            ProfileNotice.ERROR -> R.string.contributors_error
                        },
                    ),
                    style = MaterialTheme.typography.bodyLarge,
                )
                Button(onClick = onRetry) {
                    Text(stringResource(R.string.contributors_retry))
                }
            }

            else -> {
                val stored = state.read?.profile
                val roles = state.read?.roles.orEmpty()
                var displayName by remember(stored) { mutableStateOf(stored?.displayName ?: "") }
                var platform0 by remember(stored) { mutableStateOf(stored?.links?.getOrNull(0)?.platform ?: "github") }
                var handle0 by remember(stored) { mutableStateOf(stored?.links?.getOrNull(0)?.handle ?: "") }
                var platform1 by remember(stored) { mutableStateOf(stored?.links?.getOrNull(1)?.platform ?: "github") }
                var handle1 by remember(stored) { mutableStateOf(stored?.links?.getOrNull(1)?.handle ?: "") }
                var public by remember(stored) { mutableStateOf(stored?.visibility == ProfileVisibility.PUBLIC) }
                var consent by remember(stored) { mutableStateOf(stored?.publicConsent ?: false) }
                var showRoles by remember(stored) { mutableStateOf(stored?.showRoles ?: false) }
                var formError by remember { mutableStateOf<Int?>(null) }

                LazyColumn(
                    modifier = Modifier.fillMaxSize().padding(padding),
                    contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    if (stored == null) {
                        item(key = "empty") {
                            Text(
                                text = stringResource(R.string.profile_empty),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                    item(key = "name") {
                        OutlinedTextField(
                            value = displayName,
                            onValueChange = { displayName = it },
                            label = { Text(stringResource(R.string.profile_name_label)) },
                            singleLine = true,
                            modifier = Modifier
                                .fillMaxWidth()
                                .testTag(PROFILE_NAME_FIELD_TAG),
                        )
                    }
                    item(key = "link0") {
                        LinkRow(
                            platform = platform0,
                            onPlatformChange = { platform0 = it },
                            handle = handle0,
                            onHandleChange = { handle0 = it },
                        )
                    }
                    item(key = "link1") {
                        LinkRow(
                            platform = platform1,
                            onPlatformChange = { platform1 = it },
                            handle = handle1,
                            onHandleChange = { handle1 = it },
                        )
                    }
                    item(key = "visibility") {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            Switch(
                                checked = public,
                                onCheckedChange = { public = it },
                                modifier = Modifier.testTag(PROFILE_VISIBILITY_SWITCH_TAG),
                            )
                            Text(stringResource(R.string.profile_visibility_label))
                        }
                    }
                    item(key = "consent") {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            Checkbox(
                                checked = consent,
                                onCheckedChange = { consent = it },
                                modifier = Modifier.testTag(PROFILE_CONSENT_CHECK_TAG),
                            )
                            Text(
                                text = stringResource(R.string.profile_consent_text),
                                style = MaterialTheme.typography.bodySmall,
                            )
                        }
                    }
                    item(key = "show-roles") {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            Switch(checked = showRoles, onCheckedChange = { showRoles = it })
                            Text(stringResource(R.string.profile_roles_label))
                        }
                    }
                    if (roles.isNotEmpty()) {
                        item(key = "roles-list") {
                            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                                for (role in roles) {
                                    Text(
                                        text = role,
                                        style = MaterialTheme.typography.bodyMedium,
                                    )
                                }
                            }
                        }
                    }
                    formError?.let { error ->
                        item(key = "form-error") {
                            Text(
                                text = stringResource(error),
                                color = MaterialTheme.colorScheme.error,
                                style = MaterialTheme.typography.bodySmall,
                            )
                        }
                    }
                    if (state.saved) {
                        item(key = "saved") {
                            Text(
                                text = stringResource(R.string.profile_saved),
                                color = MaterialTheme.colorScheme.primary,
                                style = MaterialTheme.typography.bodySmall,
                            )
                        }
                    }
                    item(key = "save") {
                        Button(
                            onClick = {
                                val edit = ProfileEdit(
                                    displayName = displayName.trim(),
                                    links = listOf(
                                        ProfileLinkInput(platform0, handle0.trim()),
                                        ProfileLinkInput(platform1, handle1.trim()),
                                    ).filter { it.handle.isNotEmpty() },
                                    visibility = if (public) ProfileVisibility.PUBLIC else ProfileVisibility.HIDDEN,
                                    publicConsent = consent,
                                    showRoles = showRoles,
                                )
                                formError = when (profileEditError(edit)) {
                                    null -> null
                                    "display-name" -> R.string.profile_name_error
                                    "consent" -> R.string.profile_consent_error
                                    else -> R.string.profile_links_error
                                }
                                if (formError == null) onSave(edit)
                            },
                            enabled = !state.saving,
                            modifier = Modifier
                                .fillMaxWidth()
                                .testTag(PROFILE_SAVE_BUTTON_TAG),
                        ) {
                            Text(stringResource(R.string.profile_save_button))
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun LinkRow(
    platform: String,
    onPlatformChange: (String) -> Unit,
    handle: String,
    onHandleChange: (String) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    val platforms = listOf("github", "gitlab", "linkedin", "x", "instagram", "youtube", "bluesky")
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        ExposedDropdownMenuBox(expanded = expanded, onExpandedChange = { expanded = it }) {
            OutlinedTextField(
                value = platform,
                onValueChange = {},
                readOnly = true,
                label = { Text(stringResource(R.string.profile_platform_label)) },
                trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded) },
                modifier = Modifier
                    .menuAnchor()
                    .fillMaxWidth(),
            )
            ExposedDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                for (option in platforms) {
                    DropdownMenuItem(
                        text = { Text(option) },
                        onClick = {
                            onPlatformChange(option)
                            expanded = false
                        },
                    )
                }
            }
        }
        OutlinedTextField(
            value = handle,
            onValueChange = onHandleChange,
            label = { Text(stringResource(R.string.profile_handle_label)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

const val PROFILE_NAME_FIELD_TAG = "profile-name-field"
const val PROFILE_VISIBILITY_SWITCH_TAG = "profile-visibility-switch"
const val PROFILE_CONSENT_CHECK_TAG = "profile-consent-check"
const val PROFILE_SAVE_BUTTON_TAG = "profile-save-button"
