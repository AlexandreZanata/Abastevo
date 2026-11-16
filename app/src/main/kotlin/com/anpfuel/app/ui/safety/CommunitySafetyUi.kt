package com.anpfuel.app.ui.safety

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.designsystem.Button
import com.anpfuel.app.ui.designsystem.TextButton
import com.anpfuel.domain.safety.UserReportDetail
import com.anpfuel.domain.safety.UserReportReason

/** One entry of a content overflow menu. */
internal data class ContentMenuAction(val label: String, val onClick: () -> Unit)

/**
 * The "more" menu of a piece of user content: report and block live here
 * instead of loose icons (BUC-SAFE-01..03). A 48 dp touch target with a
 * spoken label; nothing renders when there is no action.
 */
@Composable
internal fun ContentOverflowMenu(
    actions: List<ContentMenuAction>,
    contentDescription: String,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    tint: Color = MaterialTheme.colorScheme.onSurfaceVariant,
) {
    if (actions.isEmpty()) return
    var open by remember { mutableStateOf(false) }
    Box(modifier = modifier) {
        IconButton(onClick = { open = true }, enabled = enabled, modifier = Modifier.size(48.dp)) {
            Icon(Icons.Default.MoreVert, contentDescription = contentDescription, tint = tint)
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            actions.forEach { action ->
                DropdownMenuItem(
                    text = { Text(action.label) },
                    onClick = {
                        open = false
                        action.onClick()
                    },
                )
            }
        }
    }
}

/**
 * Report a user (B-BR-SAFE-04): one of the six reasons plus an optional
 * private detail of at most 200 characters, counted like the server.
 */
@Composable
internal fun ReportUserDialog(
    submitting: Boolean,
    onDismiss: () -> Unit,
    onSubmit: (UserReportReason, String?) -> Unit,
) {
    var reason by rememberSaveable { mutableStateOf(UserReportReason.SPAM) }
    var detail by rememberSaveable { mutableStateOf("") }
    val remaining = UserReportDetail.remaining(detail)
    AlertDialog(
        onDismissRequest = { if (!submitting) onDismiss() },
        title = { Text(stringResource(R.string.safety_report_user_title)) },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                Text(
                    text = stringResource(R.string.safety_report_user_prompt),
                    style = MaterialTheme.typography.bodyMedium,
                )
                Column(modifier = Modifier.selectableGroup()) {
                    UserReportReason.entries.forEach { option ->
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .heightIn(min = 48.dp)
                                .selectable(
                                    selected = reason == option,
                                    enabled = !submitting,
                                    role = Role.RadioButton,
                                    onClick = { reason = option },
                                ),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            RadioButton(selected = reason == option, onClick = null, enabled = !submitting)
                            Text(
                                text = userReportReasonText(option),
                                style = MaterialTheme.typography.bodyMedium,
                                modifier = Modifier.padding(start = 12.dp),
                            )
                        }
                    }
                }
                OutlinedTextField(
                    value = detail,
                    onValueChange = { detail = it },
                    label = { Text(stringResource(R.string.safety_report_detail_hint)) },
                    supportingText = {
                        Text(
                            stringResource(
                                R.string.safety_report_detail_counter,
                                UserReportDetail.MAX_SCALARS - remaining,
                                UserReportDetail.MAX_SCALARS,
                            ),
                        )
                    },
                    isError = remaining < 0,
                    enabled = !submitting,
                    maxLines = 4,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        },
        confirmButton = {
            Button(
                onClick = { onSubmit(reason, UserReportDetail.normalize(detail)) },
                enabled = !submitting && remaining >= 0,
            ) {
                if (submitting) {
                    CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
                } else {
                    Text(stringResource(R.string.safety_report_send))
                }
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !submitting) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    )
}

/**
 * Confirms a block and says what it does (B-BR-SAFE-05): the author's
 * content is hidden for the viewer only, the author is not notified, and
 * the block can be undone under Account > Blocked users.
 */
@Composable
internal fun BlockUserDialog(
    blocking: Boolean,
    onDismiss: () -> Unit,
    onConfirm: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = { if (!blocking) onDismiss() },
        title = { Text(stringResource(R.string.safety_block_title)) },
        text = { Text(stringResource(R.string.safety_block_message)) },
        confirmButton = {
            Button(onClick = onConfirm, enabled = !blocking) {
                if (blocking) {
                    CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
                } else {
                    Text(stringResource(R.string.safety_block_confirm))
                }
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !blocking) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    )
}

@Composable
internal fun userReportReasonText(reason: UserReportReason): String = stringResource(
    when (reason) {
        UserReportReason.SPAM -> R.string.safety_reason_spam
        UserReportReason.HARASSMENT -> R.string.safety_reason_harassment
        UserReportReason.HATE -> R.string.safety_reason_hate
        UserReportReason.IMPERSONATION -> R.string.safety_reason_impersonation
        UserReportReason.ILLEGAL -> R.string.safety_reason_illegal
        UserReportReason.OTHER -> R.string.safety_reason_other
    },
)
