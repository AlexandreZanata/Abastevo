package com.anpfuel.app.capture

import androidx.compose.foundation.layout.Column
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.anpfuel.app.R

/** Visible receipt feedback; local persistence never claims server acceptance. */
@Composable
internal fun PhotoSubmissionFeedback(
    submit: CaptureOcrViewModel.SubmitState?,
    onReturnCommunity: () -> Unit,
) {
    if (submit !is CaptureOcrViewModel.SubmitState.Queued &&
        submit !is CaptureOcrViewModel.SubmitState.Sent &&
        submit !is CaptureOcrViewModel.SubmitState.Partial) return
    AlertDialog(
        onDismissRequest = onReturnCommunity,
        title = { Text(stringResource(when (submit) {
            is CaptureOcrViewModel.SubmitState.Sent -> R.string.capture_sent_title
            is CaptureOcrViewModel.SubmitState.Partial -> R.string.capture_partial_title
            else -> R.string.capture_saved_title
        })) },
        text = {
            Column {
                when (submit) {
                    is CaptureOcrViewModel.SubmitState.Queued -> {
                        Text(stringResource(R.string.capture_sent_count, submit.count))
                        Text(stringResource(if (submit.retrying) R.string.capture_queue_retrying
                            else R.string.capture_queue_background))
                    }
                    is CaptureOcrViewModel.SubmitState.Sent -> {
                        Text(stringResource(R.string.capture_sent, submit.count))
                        if (submit.pendingValidation) Text(stringResource(R.string.capture_sent_pending_validation))
                    }
                    is CaptureOcrViewModel.SubmitState.Partial ->
                        Text(stringResource(R.string.capture_partial, submit.sent, submit.reason))
                    else -> Unit
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onReturnCommunity) { Text(stringResource(R.string.capture_back_community)) }
        },
    )
}
