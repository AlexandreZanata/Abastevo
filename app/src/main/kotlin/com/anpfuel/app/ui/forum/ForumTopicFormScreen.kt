package com.anpfuel.app.ui.forum

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumTopicReceipt

private val TopicAvailableCategories = listOf(
    ForumCategory.FUELS,
    ForumCategory.MARKET,
    ForumCategory.REGULATION,
    ForumCategory.CONSUMPTION,
    ForumCategory.ROADS_MOBILITY,
    ForumCategory.DRIVER_ECONOMY,
    ForumCategory.ABASTEVO_DATA,
)

private val TopicAvailableFuels = listOf(
    "GASOLINE_REGULAR",
    "GASOLINE_PREMIUM",
    "ETHANOL",
    "DIESEL_S10",
    "DIESEL_S500",
    "GLP",
    "GNV",
)

@Composable
fun ForumTopicFormScreen(
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    onSignIn: () -> Unit = {},
    onTopicCreated: (ForumTopicReceipt) -> Unit = {},
    onTopicEdited: (ForumTopicReceipt) -> Unit = {},
    onTopicRemoved: () -> Unit = {},
    viewModel: ForumTopicFormViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    BackHandler(enabled = true) {
        viewModel.onBackPressed(onBack)
    }

    if (state.requireAuthPrompt) {
        viewModel.dismissAuthPrompt()
        onSignIn()
    }

    LaunchedEffect(state.isRemoved) {
        if (state.isRemoved) onTopicRemoved()
    }

    ForumTopicFormContent(
        state = state,
        onBack = { viewModel.onBackPressed(onBack) },
        onConfirmDiscard = { viewModel.confirmDiscard(onBack) },
        onDismissDiscard = viewModel::dismissDiscardDialog,
        onUpdateTitle = viewModel::updateTitle,
        onUpdateBody = viewModel::updateBodyText,
        onUpdateCategory = viewModel::updateCategory,
        onUpdateUf = viewModel::updateUf,
        onUpdateIbge = viewModel::updateMunicipalityIbge,
        onUpdateFuel = viewModel::updateFuelProduct,
        onUpdateAlias = viewModel::updatePublicAlias,
        onSubmit = viewModel::submit,
        onConfirmRemove = viewModel::confirmRemove,
        onDismissRemove = viewModel::dismissDeleteDialog,
        onExecuteRemove = viewModel::executeRemove,
        onDismissActionError = viewModel::dismissActionError,
        onDismissSubmissionReceipt = {
            val receipt = state.submissionReceipt
            viewModel.dismissSubmissionReceipt()
            if (receipt != null) {
                if (state.isEditMode) {
                    onTopicEdited(receipt)
                } else {
                    onTopicCreated(receipt)
                }
            }
        },
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun ForumTopicFormContent(
    state: ForumTopicFormUiState,
    onBack: () -> Unit,
    onConfirmDiscard: () -> Unit,
    onDismissDiscard: () -> Unit,
    onUpdateTitle: (String) -> Unit,
    onUpdateBody: (String) -> Unit,
    onUpdateCategory: (ForumCategory?) -> Unit,
    onUpdateUf: (String?) -> Unit,
    onUpdateIbge: (String) -> Unit,
    onUpdateFuel: (String?) -> Unit,
    onUpdateAlias: (String) -> Unit,
    onSubmit: () -> Unit,
    onConfirmRemove: () -> Unit,
    onDismissRemove: () -> Unit,
    onExecuteRemove: () -> Unit,
    onDismissActionError: () -> Unit,
    onDismissSubmissionReceipt: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            AnpTopAppBar(
                title = {
                    Text(
                        stringResource(
                            if (state.isEditMode) R.string.forum_edit_topic else R.string.forum_new_topic
                        )
                    )
                },
                onNavigateUp = onBack,
            )
        },
    ) { padding ->
        if (state.isLoading) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding),
                verticalArrangement = Arrangement.Center,
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                CircularProgressIndicator(modifier = Modifier.size(36.dp))
            }
            return@AnpScaffold
        }

        if (state.notFoundError) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding)
                    .padding(24.dp),
                verticalArrangement = Arrangement.Center,
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text(
                    text = stringResource(R.string.forum_topic_not_found),
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(16.dp))
                Button(onClick = onBack) {
                    Text(stringResource(R.string.forum_retry))
                }
            }
            return@AnpScaffold
        }

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            // Action Error Banner
            val error = state.actionError
            if (error != null) {
                item {
                    val containerColor = MaterialTheme.colorScheme.errorContainer
                    Card(
                        modifier = Modifier.fillMaxWidth(),
                        shape = AnpContainerDefaults.containerShape(),
                        colors = CardDefaults.cardColors(containerColor = containerColor),
                    ) {
                        Column(modifier = Modifier.padding(16.dp)) {
                            val errorMessage = when (error) {
                                is ForumActionError.Conflict -> stringResource(R.string.forum_conflict_error)
                                is ForumActionError.RateLimited -> {
                                    val secs = error.retryAfterSeconds ?: 30L
                                    stringResource(R.string.forum_rate_limited_error, secs)
                                }
                                is ForumActionError.Offline -> stringResource(R.string.forum_offline_notice)
                                is ForumActionError.TargetMissing -> stringResource(R.string.forum_post_not_found_title)
                                is ForumActionError.Generic -> error.message ?: stringResource(R.string.forum_unavailable_title)
                            }
                            Text(
                                text = errorMessage,
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onErrorContainer,
                            )
                            Spacer(Modifier.height(8.dp))
                            TextButton(onClick = onDismissActionError) {
                                Text(stringResource(R.string.forum_external_link_dialog_cancel))
                            }
                        }
                    }
                }
            }

            // Title field
            item {
                OutlinedTextField(
                    value = state.title,
                    onValueChange = onUpdateTitle,
                    label = { Text(stringResource(R.string.forum_topic_title_label)) },
                    placeholder = { Text(stringResource(R.string.forum_topic_title_hint)) },
                    supportingText = {
                        val count = state.title.length
                        val errText = when (state.validationError) {
                            ForumTopicValidationError.TITLE_EMPTY -> stringResource(R.string.forum_topic_validation_title_empty)
                            ForumTopicValidationError.TITLE_TOO_LONG -> stringResource(R.string.forum_topic_validation_title_too_long)
                            else -> null
                        }
                        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                            Text(errText.orEmpty(), color = MaterialTheme.colorScheme.error)
                            Text(stringResource(R.string.forum_topic_title_counter, count))
                        }
                    },
                    isError = state.validationError == ForumTopicValidationError.TITLE_EMPTY ||
                        state.validationError == ForumTopicValidationError.TITLE_TOO_LONG,
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(
                        capitalization = KeyboardCapitalization.Sentences,
                        imeAction = ImeAction.Next,
                    ),
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            // Category selector
            item {
                Column {
                    Text(
                        text = stringResource(R.string.forum_topic_category_label),
                        style = MaterialTheme.typography.labelMedium,
                        fontWeight = FontWeight.SemiBold,
                    )
                    if (state.validationError == ForumTopicValidationError.CATEGORY_REQUIRED) {
                        Text(
                            text = stringResource(R.string.forum_topic_validation_category),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.error,
                        )
                    }
                    Spacer(Modifier.height(8.dp))
                    LazyRow(
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        items(TopicAvailableCategories, key = { it.wireCode }) { cat ->
                            val selected = state.selectedCategory == cat
                            FilterChip(
                                selected = selected,
                                onClick = { onUpdateCategory(if (selected) null else cat) },
                                label = { Text(cat.wireCode) },
                            )
                        }
                    }
                }
            }

            // Content body field
            item {
                OutlinedTextField(
                    value = state.bodyText,
                    onValueChange = onUpdateBody,
                    label = { Text(stringResource(R.string.forum_topic_body_label)) },
                    placeholder = { Text(stringResource(R.string.forum_topic_body_hint)) },
                    supportingText = {
                        val count = state.bodyText.length
                        val errText = when (state.validationError) {
                            ForumTopicValidationError.BODY_EMPTY -> stringResource(R.string.forum_topic_validation_body_empty)
                            ForumTopicValidationError.BODY_TOO_LONG -> stringResource(R.string.forum_topic_validation_body_too_long)
                            else -> null
                        }
                        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                            Text(errText.orEmpty(), color = MaterialTheme.colorScheme.error)
                            Text(stringResource(R.string.forum_topic_body_counter, count))
                        }
                    },
                    isError = state.validationError == ForumTopicValidationError.BODY_EMPTY ||
                        state.validationError == ForumTopicValidationError.BODY_TOO_LONG,
                    minLines = 5,
                    maxLines = 15,
                    keyboardOptions = KeyboardOptions(
                        capitalization = KeyboardCapitalization.Sentences,
                        imeAction = ImeAction.Default,
                    ),
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            // Optional Regional Scope
            item {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    OutlinedTextField(
                        value = state.selectedUf.orEmpty(),
                        onValueChange = { onUpdateUf(it.take(2)) },
                        label = { Text(stringResource(R.string.forum_topic_region_label)) },
                        placeholder = { Text("SP") },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(
                            capitalization = KeyboardCapitalization.Characters,
                            imeAction = ImeAction.Next,
                        ),
                        modifier = Modifier.weight(1f),
                    )
                    OutlinedTextField(
                        value = state.municipalityIbge,
                        onValueChange = onUpdateIbge,
                        label = { Text(stringResource(R.string.forum_topic_municipality_label)) },
                        placeholder = { Text("3550308") },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(
                            keyboardType = KeyboardType.Number,
                            imeAction = ImeAction.Next,
                        ),
                        modifier = Modifier.weight(1.5f),
                    )
                }
            }

            // Optional Fuel Product Selection
            item {
                Column {
                    Text(
                        text = stringResource(R.string.forum_topic_fuel_label),
                        style = MaterialTheme.typography.labelMedium,
                    )
                    Spacer(Modifier.height(6.dp))
                    LazyRow(
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        items(TopicAvailableFuels, key = { it }) { fuel ->
                            val selected = state.selectedFuelProduct == fuel
                            FilterChip(
                                selected = selected,
                                onClick = { onUpdateFuel(if (selected) null else fuel) },
                                label = { Text(fuel) },
                            )
                        }
                    }
                }
            }

            // Optional Public Alias
            item {
                OutlinedTextField(
                    value = state.publicAlias,
                    onValueChange = onUpdateAlias,
                    label = { Text(stringResource(R.string.forum_topic_alias_label)) },
                    placeholder = { Text(stringResource(R.string.forum_topic_alias_hint)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(
                        capitalization = KeyboardCapitalization.Words,
                        imeAction = ImeAction.Done,
                    ),
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            // Action Buttons
            item {
                Spacer(Modifier.height(8.dp))
                Button(
                    onClick = onSubmit,
                    enabled = !state.isSubmitting && !state.isRemoving,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    if (state.isSubmitting) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(20.dp),
                            color = MaterialTheme.colorScheme.onPrimary,
                            strokeWidth = 2.dp,
                        )
                        Spacer(Modifier.size(8.dp))
                        Text(stringResource(R.string.forum_topic_submitting))
                    } else {
                        Text(
                            stringResource(
                                if (state.isEditMode) R.string.forum_topic_save_changes else R.string.forum_topic_publish
                            )
                        )
                    }
                }

                if (state.isEditMode) {
                    Spacer(Modifier.height(12.dp))
                    OutlinedButton(
                        onClick = onConfirmRemove,
                        enabled = !state.isSubmitting && !state.isRemoving,
                        colors = ButtonDefaults.outlinedButtonColors(
                            contentColor = MaterialTheme.colorScheme.error,
                        ),
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        if (state.isRemoving) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(20.dp),
                                color = MaterialTheme.colorScheme.error,
                                strokeWidth = 2.dp,
                            )
                        } else {
                            Text(stringResource(R.string.forum_delete_topic))
                        }
                    }
                }
            }
        }
    }

    // Discard draft confirmation dialog
    if (state.showDiscardDialog) {
        AlertDialog(
            onDismissRequest = onDismissDiscard,
            title = { Text(stringResource(R.string.forum_topic_discard_dialog_title)) },
            text = { Text(stringResource(R.string.forum_topic_discard_dialog_message)) },
            confirmButton = {
                TextButton(onClick = onConfirmDiscard) {
                    Text(
                        stringResource(R.string.forum_topic_discard_confirm),
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            },
            dismissButton = {
                TextButton(onClick = onDismissDiscard) {
                    Text(stringResource(R.string.forum_topic_discard_cancel))
                }
            },
        )
    }

    // Delete topic confirmation dialog
    if (state.showDeleteDialog) {
        AlertDialog(
            onDismissRequest = onDismissRemove,
            title = { Text(stringResource(R.string.forum_delete_topic_dialog_title)) },
            text = { Text(stringResource(R.string.forum_delete_topic_dialog_message)) },
            confirmButton = {
                TextButton(onClick = onExecuteRemove) {
                    Text(
                        stringResource(R.string.forum_delete_topic),
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            },
            dismissButton = {
                TextButton(onClick = onDismissRemove) {
                    Text(stringResource(R.string.forum_external_link_dialog_cancel))
                }
            },
        )
    }

    // Submission Receipt Dialog
    if (state.submissionReceipt != null) {
        AlertDialog(
            onDismissRequest = onDismissSubmissionReceipt,
            title = { Text(stringResource(R.string.forum_detail_title)) },
            text = {
                Column {
                    Text(stringResource(R.string.forum_topic_pending_receipt))
                    Spacer(Modifier.height(8.dp))
                    Text(
                        text = "ID: ${state.submissionReceipt.id}",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            },
            confirmButton = {
                Button(onClick = onDismissSubmissionReceipt) {
                    Text("OK")
                }
            },
        )
    }
}
