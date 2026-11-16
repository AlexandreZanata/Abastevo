package com.anpfuel.app.ui.enterprise.attachment

import android.Manifest
import android.content.pm.PackageManager
import android.graphics.BitmapFactory
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.app.BuildConfig
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.application.enterprise.attachment.EnterpriseReceipts
import com.anpfuel.application.enterprise.attachment.ReceiptCaptureController
import com.anpfuel.application.enterprise.attachment.ReceiptCaptureState
import com.anpfuel.application.enterprise.attachment.ReceiptDraft
import com.anpfuel.application.enterprise.attachment.ReceiptOriginals
import com.anpfuel.domain.enterprise.attachment.AttachmentLabel
import com.anpfuel.domain.enterprise.attachment.AttachmentLocalFailure
import com.anpfuel.domain.enterprise.attachment.AttachmentRefusal
import com.anpfuel.domain.enterprise.attachment.AttachmentView
import com.anpfuel.domain.enterprise.attachment.LegibilityIssue
import com.anpfuel.domain.enterprise.attachment.ReceiptRefusal
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/*
 * SCR-DOC-01 (EMP-DOC-004, B-BR-EMP-450..454, EDR-291..293): bring an original in with the system camera or the photo
 * picker, show the original and its legibility hint, and hand an unsaved photo to the record's own flow. Nothing here
 * persists a photo and nothing claims that a receipt is valid.
 */

/** The capture sheet's owner; the unsaved photo lives here, in memory only (B-BR-EMP-453). */
@HiltViewModel
class ReceiptCaptureViewModel(originals: ReceiptOriginals, queueEnabled: Boolean) : ViewModel() {
    @Inject constructor(originals: ReceiptOriginals, receipts: EnterpriseReceipts) : this(originals, receipts.enabled)
    val controller = ReceiptCaptureController(originals)
    val state = controller.state
    /** Shown only with the attachment queue on and the rollback switch on (B-BR-EMP-450). */
    val enabled: Boolean = BuildConfig.ENTERPRISE_RECEIPT_CAPTURE_UI_ENABLED && queueEnabled
    suspend fun cameraTarget(): String = controller.cameraTarget()
    fun cameraResult(reference: String, taken: Boolean) { viewModelScope.launch { controller.cameraResult(reference, taken) } }
    fun imported(reference: String?) { viewModelScope.launch { controller.imported(reference) } }
    fun cameraDenied() = controller.cameraDenied()
    fun reset() = controller.reset()
    override fun onCleared() { controller.reset(); super.onCleared() }
}

/** How a sheet asks the platform for a photo; the device test substitutes a synthetic source. */
interface ReceiptPickers {
    fun camera()
    fun library()
}

/** What a record screen needs to offer capture; null when capture is switched off. */
class ReceiptCaptureUi(val state: ReceiptCaptureState, val pickers: ReceiptPickers, val reset: () -> Unit)

/** The system camera (after the CAMERA permission) and the photo picker (no storage permission), EDR-291. */
@Composable
fun rememberSystemReceiptPickers(model: ReceiptCaptureViewModel): ReceiptPickers {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var pending by rememberSaveable { mutableStateOf<String?>(null) }
    val take = rememberLauncherForActivityResult(ActivityResultContracts.TakePicture()) { taken ->
        val target = pending
        pending = null
        if (target != null) model.cameraResult(target, taken)
    }
    val launchCamera: () -> Unit = {
        scope.launch {
            val target = try { model.cameraTarget() } catch (_: Exception) { null } ?: return@launch
            pending = target
            try { take.launch(Uri.parse(target)) } catch (_: Exception) { pending = null; model.cameraResult(target, false) }
        }
    }
    val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (granted) launchCamera() else model.cameraDenied()
    }
    val pick = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri -> model.imported(uri?.toString()) }
    return remember(model) {
        object : ReceiptPickers {
            override fun camera() {
                if (ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) launchCamera()
                else permission.launch(Manifest.permission.CAMERA)
            }
            override fun library() {
                try { pick.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) } catch (_: Exception) { model.imported(null) }
            }
        }
    }
}

private fun issueText(issue: LegibilityIssue): Int = when (issue) {
    LegibilityIssue.LOW_RESOLUTION -> R.string.enterprise_receipts_hint_low_resolution
    LegibilityIssue.TOO_DARK -> R.string.enterprise_receipts_hint_dark
    LegibilityIssue.TOO_BRIGHT -> R.string.enterprise_receipts_hint_bright
    LegibilityIssue.LOW_CONTRAST -> R.string.enterprise_receipts_hint_contrast
    LegibilityIssue.BLURRED -> R.string.enterprise_receipts_hint_blurred
}

private fun refusalText(refusal: ReceiptRefusal): Int = when (refusal) {
    ReceiptRefusal.MISSING -> R.string.enterprise_receipts_refused_missing
    ReceiptRefusal.TOO_LARGE -> R.string.enterprise_receipts_refused_too_large
    ReceiptRefusal.UNSUPPORTED -> R.string.enterprise_receipts_refused_unsupported
}

/** The reason of a refused save on the device; nothing was saved. */
fun localFailureText(failure: AttachmentLocalFailure): Int = when (failure) {
    AttachmentLocalFailure.UNKNOWN_OPERATION -> R.string.enterprise_receipts_local_unknown_record
    AttachmentLocalFailure.FULL -> R.string.enterprise_receipts_local_full
    AttachmentLocalFailure.BUDGET -> R.string.enterprise_receipts_local_budget
    AttachmentLocalFailure.FOREIGN -> R.string.enterprise_receipts_local_foreign
    AttachmentLocalFailure.DISABLED -> R.string.enterprise_receipts_local_disabled
    AttachmentLocalFailure.NOT_JPEG, AttachmentLocalFailure.SIZE, AttachmentLocalFailure.DUPLICATE -> R.string.enterprise_receipts_local_invalid
}

private fun labelText(label: AttachmentLabel): Int = when (label) {
    AttachmentLabel.SAVED_ON_DEVICE -> R.string.enterprise_receipts_label_saved
    AttachmentLabel.SENDING -> R.string.enterprise_receipts_label_sending
    AttachmentLabel.VERIFYING -> R.string.enterprise_receipts_label_verifying
    AttachmentLabel.VERIFIED -> R.string.enterprise_receipts_label_stored
    AttachmentLabel.REFUSED -> R.string.enterprise_receipts_label_refused
    AttachmentLabel.FILE_MISSING -> R.string.enterprise_receipts_label_missing
    AttachmentLabel.NEEDS_ATTENTION -> R.string.enterprise_receipts_label_attention
}

private fun attachmentRefusalText(refusal: AttachmentRefusal): Int = when (refusal) {
    AttachmentRefusal.OWNER_REJECTED -> R.string.enterprise_receipts_refusal_owner
    AttachmentRefusal.FORBIDDEN -> R.string.enterprise_receipts_refusal_forbidden
    AttachmentRefusal.VALIDATION_FAILED -> R.string.enterprise_receipts_refusal_validation
    AttachmentRefusal.CONFLICT -> R.string.enterprise_receipts_refusal_conflict
    AttachmentRefusal.OPERATION_ID_REUSED -> R.string.enterprise_receipts_refusal_reused
    AttachmentRefusal.QUARANTINED -> R.string.enterprise_receipts_refusal_quarantined
    AttachmentRefusal.FAILED -> R.string.enterprise_receipts_refusal_failed
}

/** SCR-DOC-01: the original, its hint, keep or retake, and "Salvar no aparelho" only for an accepted original. */
@Composable
fun ReceiptCaptureCard(title: String, capture: ReceiptCaptureUi, busy: Boolean, onSave: (ReceiptDraft) -> Unit, onCancel: () -> Unit,
    modifier: Modifier = Modifier) {
    val state = capture.state
    ReceiptContainer(modifier.testTag("enterprise-receipt-capture")) {
        Text(title, style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.enterprise_receipts_synthetic_notice), style = MaterialTheme.typography.bodySmall)
        Button(onClick = capture.pickers::camera, enabled = !busy && !state.reading,
            modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-receipt-camera")) {
            Text(stringResource(R.string.enterprise_receipts_camera))
        }
        OutlinedButton(onClick = capture.pickers::library, enabled = !busy && !state.reading,
            modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-receipt-import")) {
            Text(stringResource(R.string.enterprise_receipts_import))
        }
        if (state.reading) {
            CircularProgressIndicator()
            Notice(stringResource(R.string.enterprise_receipts_reading))
        }
        val refusal = state.refusal
        val message = when {
            state.cameraDenied -> R.string.enterprise_receipts_camera_denied
            refusal != null -> refusalText(refusal)
            state.cancelled -> R.string.enterprise_receipts_cancelled
            else -> null
        }
        message?.let { Notice(stringResource(it), Modifier.testTag("enterprise-receipt-message")) }
        state.draft?.let { draft ->
            Preview(draft)
            val hints = if (!draft.hint.checked) listOf(stringResource(R.string.enterprise_receipts_hint_not_checked))
                else if (draft.hint.looksSharp) listOf(stringResource(R.string.enterprise_receipts_hint_sharp))
                else draft.hint.issues.map { stringResource(issueText(it)) }
            val disclaimer = stringResource(R.string.enterprise_receipts_hint_disclaimer)
            Column(Modifier.testTag("enterprise-receipt-hint").semantics(mergeDescendants = true) {
                stateDescription = (hints + disclaimer).joinToString(" ")
            }, verticalArrangement = Arrangement.spacedBy(4.dp)) {
                hints.forEach { Text(it) }
                Text(disclaimer, style = MaterialTheme.typography.bodySmall)
            }
            Text(stringResource(R.string.enterprise_receipts_keep_or_retake), style = MaterialTheme.typography.bodySmall)
        }
        Button(onClick = { state.draft?.let(onSave) }, enabled = !busy && !state.reading && state.draft != null,
            modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).testTag("enterprise-receipt-save")) {
            Text(stringResource(R.string.enterprise_receipts_save))
        }
        TextButton(onClick = onCancel, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("enterprise-receipt-cancel")) {
            Text(stringResource(R.string.enterprise_receipts_cancel))
        }
    }
}

/** A bounded preview of the unsaved original; decoding never changes the bytes that will be saved. */
@Composable
private fun Preview(draft: ReceiptDraft) {
    var image by remember(draft) { mutableStateOf<ImageBitmap?>(null) }
    LaunchedEffect(draft) { image = withContext(Dispatchers.Default) { previewOf(draft) } }
    val description = stringResource(R.string.enterprise_receipts_preview_description)
    image?.let {
        Image(it, contentDescription = description, contentScale = ContentScale.Fit,
            modifier = Modifier.fillMaxWidth().heightIn(max = 280.dp).testTag("enterprise-receipt-preview"))
    }
}

private fun previewOf(draft: ReceiptDraft): ImageBitmap? {
    val bytes = draft.original.bytes()
    return try {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        var scale = 1
        while (maxOf(bounds.outWidth, bounds.outHeight) / (scale * 2) >= PREVIEW_SIDE) scale *= 2
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, BitmapFactory.Options().apply { inSampleSize = scale })?.asImageBitmap()
    } catch (_: Exception) { null } finally { bytes.fill(0) }
}

/** Each attachment of one record with its own label, never the record's (B-BR-EMP-454). */
@Composable
fun AttachmentRows(views: List<AttachmentView>, tag: String, onRetake: (() -> Unit)?) {
    if (views.isEmpty()) return
    Text(stringResource(R.string.enterprise_receipts_photos), style = MaterialTheme.typography.labelLarge)
    views.forEachIndexed { index, view ->
        val label = stringResource(labelText(view.label))
        val reason = view.refusal?.takeIf { view.label == AttachmentLabel.REFUSED }?.let { stringResource(attachmentRefusalText(it)) }
        Column(Modifier.testTag("$tag-$index").semantics(mergeDescendants = true) {
            contentDescription = listOfNotNull(label, reason).joinToString(". ")
        }) {
            Text(stringResource(R.string.enterprise_receipts_photo_status, index + 1, label))
            reason?.let { Text(stringResource(R.string.enterprise_receipts_reason, it)) }
        }
        if (view.label == AttachmentLabel.FILE_MISSING && onRetake != null) {
            OutlinedButton(onClick = onRetake, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("$tag-$index-retake")) {
                Text(stringResource(R.string.enterprise_receipts_retake))
            }
        }
    }
}

@Composable
private fun ReceiptContainer(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    val fill = MaterialTheme.colorScheme.surfaceContainerLow
    Card(modifier.fillMaxWidth(), shape = AnpContainerDefaults.containerShape(),
        colors = CardDefaults.cardColors(containerColor = fill), border = AnpContainerDefaults.borderStroke(fill)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) { content() }
    }
}

@Composable
private fun Notice(message: String, modifier: Modifier = Modifier) {
    Text(message, modifier = modifier.semantics { liveRegion = LiveRegionMode.Polite })
}

private const val PREVIEW_SIDE = 1024
