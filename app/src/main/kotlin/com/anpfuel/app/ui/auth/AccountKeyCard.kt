package com.anpfuel.app.ui.auth

import android.app.Activity
import android.app.KeyguardManager
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.ContextWrapper
import android.content.Intent
import android.os.Build
import android.os.PersistableBundle
import android.view.Window
import android.view.WindowManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.anpfuel.app.R
import com.anpfuel.application.portable.AuthFlow
import java.util.WeakHashMap
import kotlinx.coroutines.delay

private enum class KeyAction { REVEAL, COPY, SHARE, SAVE }

/** Shared signup/profile backup. No secret enters saved instance state or app files. */
@Composable
fun AccountKeyCard(backup: AuthFlow.KeyBackup, modifier: Modifier = Modifier) {
    SecureAccountWindow()
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    val clipboard = remember(context) { context.getSystemService(ClipboardManager::class.java) }
    var revealed by remember(backup) { mutableStateOf(false) }
    var confirmation by remember(backup) { mutableStateOf<KeyAction?>(null) }
    var pending by remember(backup) { mutableStateOf<KeyAction?>(null) }
    var copied by remember(backup) { mutableStateOf(false) }
    var notice by remember(backup) { mutableStateOf<Int?>(null) }
    var needsDeviceLock by remember { mutableStateOf(false) }

    fun clearOwnedClipboard() {
        val clip = clipboard.primaryClip
        if (clip != null && clip.itemCount == 1 && clip.getItemAt(0).text?.toString() == backup.accountKey) {
            if (Build.VERSION.SDK_INT >= 28) clipboard.clearPrimaryClip()
            else clipboard.setPrimaryClip(ClipData.newPlainText("", ""))
        }
    }

    val saveFile = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("text/plain")) { uri ->
        if (uri != null) {
            try {
                val output = context.contentResolver.openOutputStream(uri) ?: error("unavailable")
                output.use { it.write(backup.accountKey.toByteArray(Charsets.UTF_8)) }
                notice = R.string.account_key_saved
            } catch (_: Exception) {
                notice = R.string.account_key_export_failed
            }
        }
    }

    fun perform(action: KeyAction) {
        when (action) {
            KeyAction.REVEAL -> revealed = true
            KeyAction.COPY -> {
                val clip = ClipData.newPlainText(context.getString(R.string.auth_key_label), backup.accountKey)
                clip.description.extras = PersistableBundle().apply {
                    putBoolean("android.content.extra.IS_SENSITIVE", true)
                }
                clipboard.setPrimaryClip(clip)
                copied = true
                notice = R.string.account_key_copied
            }
            KeyAction.SAVE -> saveFile.launch("abastevo-account-key.txt")
            KeyAction.SHARE -> {
                val send = Intent(Intent.ACTION_SEND).apply {
                    type = "text/plain"
                    putExtra(Intent.EXTRA_TEXT, backup.accountKey)
                }
                try {
                    context.startActivity(Intent.createChooser(send, context.getString(R.string.account_key_share_title)))
                } catch (_: Exception) {
                    notice = R.string.account_key_export_failed
                }
            }
        }
    }

    val unlock = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        val action = pending
        pending = null
        if (result.resultCode == Activity.RESULT_OK && action != null) perform(action)
    }
    fun authorize(action: KeyAction) {
        val manager = context.getSystemService(KeyguardManager::class.java)
        if (!manager.isDeviceSecure) {
            needsDeviceLock = true
            return
        }
        @Suppress("DEPRECATION")
        val intent = manager.createConfirmDeviceCredentialIntent(
            context.getString(R.string.account_key_unlock_title),
            context.getString(R.string.account_key_unlock_copy),
        )
        if (intent == null) {
            notice = R.string.account_key_export_failed
            return
        }
        pending = action
        try {
            unlock.launch(intent)
        } catch (_: Exception) {
            pending = null
            notice = R.string.account_key_export_failed
        }
    }

    DisposableEffect(lifecycleOwner, backup) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_STOP) {
                revealed = false
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose {
            lifecycleOwner.lifecycle.removeObserver(observer)
            clearOwnedClipboard()
        }
    }
    LaunchedEffect(revealed) {
        if (revealed) {
            delay(60_000L)
            revealed = false
        }
    }
    LaunchedEffect(copied) {
        if (copied) {
            delay(60_000L)
            clearOwnedClipboard()
            copied = false
        }
    }

    Card(
        modifier = modifier.fillMaxWidth(), shape = RoundedCornerShape(24.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
    ) {
        Column(Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Icon(Icons.Default.Lock, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
                Text(stringResource(R.string.profile_backup_title), style = MaterialTheme.typography.titleLarge,
                    modifier = Modifier.semantics { heading() })
            }
            Text(stringResource(R.string.profile_backup_username, backup.username),
                style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
            Text(stringResource(R.string.profile_backup_subtitle), style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
            Surface(shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.surfaceContainerHighest) {
                Text(if (revealed) backup.accountKey.chunked(4).joinToString(" ") else "•••• •••• •••• ••••",
                    modifier = Modifier.fillMaxWidth().padding(20.dp),
                    style = MaterialTheme.typography.titleMedium.copy(fontFamily = FontFamily.Monospace))
            }
            OutlinedButton(
                onClick = { if (revealed) revealed = false else authorize(KeyAction.REVEAL) },
                enabled = pending == null, modifier = Modifier.fillMaxWidth(),
            ) {
                Text(stringResource(if (revealed) R.string.profile_backup_hide else R.string.profile_backup_show))
            }
            OutlinedButton(onClick = { confirmation = KeyAction.COPY }, modifier = Modifier.fillMaxWidth(), enabled = pending == null) {
                Text(stringResource(R.string.auth_copy_key))
            }
            Button(onClick = { confirmation = KeyAction.SAVE }, modifier = Modifier.fillMaxWidth(), enabled = pending == null) {
                Text(stringResource(R.string.account_key_save_file))
            }
            TextButton(onClick = { confirmation = KeyAction.SHARE }, modifier = Modifier.fillMaxWidth(), enabled = pending == null) {
                Text(stringResource(R.string.account_key_share_title))
            }
            notice?.let { Text(stringResource(it), style = MaterialTheme.typography.bodySmall) }
        }
    }
    confirmation?.let { action ->
        AlertDialog(
            onDismissRequest = { confirmation = null },
            title = { Text(stringResource(R.string.account_key_export_title)) },
            text = { Text(stringResource(R.string.account_key_export_warning)) },
            confirmButton = {
                TextButton(onClick = { confirmation = null; authorize(action) }) {
                    Text(stringResource(R.string.auth_continue))
                }
            },
            dismissButton = { TextButton(onClick = { confirmation = null }) { Text(stringResource(R.string.action_cancel)) } },
        )
    }
    if (needsDeviceLock) {
        AlertDialog(
            onDismissRequest = { needsDeviceLock = false },
            title = { Text(stringResource(R.string.account_key_unlock_title)) },
            text = { Text(stringResource(R.string.account_key_device_lock_required)) },
            confirmButton = { TextButton(onClick = { needsDeviceLock = false }) { Text(stringResource(R.string.auth_continue)) } },
        )
    }
}

private fun Context.activity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.activity()
    else -> null
}

/** Reference counted for overlapping navigation compositions; preserve existing flags. */
private object SecureWindows {
    data class Entry(var users: Int, val alreadySecure: Boolean)
    val entries = WeakHashMap<Window, Entry>()
}

@Composable
fun SecureAccountWindow() {
    val window = LocalContext.current.activity()?.window
    DisposableEffect(window) {
        if (window != null) {
            val entry = SecureWindows.entries.getOrPut(window) {
                SecureWindows.Entry(0, window.attributes.flags and WindowManager.LayoutParams.FLAG_SECURE != 0)
            }
            entry.users++
            window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        }
        onDispose {
            if (window != null) {
                val entry = SecureWindows.entries[window]
                if (entry != null && --entry.users == 0) {
                    if (!entry.alreadySecure) window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE)
                    SecureWindows.entries.remove(window)
                }
            }
        }
    }
}
