package com.anpfuel.app.ui.stations

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.ViewModel
import com.anpfuel.app.R
import com.anpfuel.application.portable.AuthApiResult
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.data.remote.StationIntakeHttpClient
import com.anpfuel.domain.discovery.ServerStation
import dagger.hilt.android.lifecycle.HiltViewModel
import java.io.IOException
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONException
import org.json.JSONObject

private enum class CorrectionField(val wire: String, @StringRes val label: Int) {
    Name("display_name", R.string.suggest_display),
    Address("address", R.string.station_correction_address),
    Closed("operating_status", R.string.station_correction_closed),
}

internal enum class CorrectionSubmission { Sent, SignIn, Failed }

@HiltViewModel
internal class StationCorrectionViewModel @Inject constructor(
    private val auth: AuthFlow,
    private val client: StationIntakeHttpClient,
) : ViewModel() {
    suspend fun submit(stationId: String, key: String, field: String, value: JSONObject): CorrectionSubmission =
        withContext(Dispatchers.IO) {
            try {
                val session = when (val result = auth.refreshSession()) {
                    is AuthApiResult.Ok -> result.value ?: return@withContext CorrectionSubmission.SignIn
                    is AuthApiResult.Err -> return@withContext CorrectionSubmission.Failed
                }
                client.submitCorrection(session.familyId, session.accessToken, stationId, key, field, value)
                CorrectionSubmission.Sent
            } catch (_: IOException) {
                CorrectionSubmission.Failed
            } catch (_: JSONException) {
                CorrectionSubmission.Failed
            }
        }
}

@Composable
internal fun StationCorrectionDialog(
    station: ServerStation,
    onDismiss: () -> Unit,
    onSignIn: () -> Unit,
    viewModel: StationCorrectionViewModel = hiltViewModel(),
) {
    var field by rememberSaveable(station.stationId) { mutableStateOf(CorrectionField.Name) }
    var value by rememberSaveable(station.stationId) { mutableStateOf("") }
    var key by rememberSaveable(station.stationId) { mutableStateOf(UUID.randomUUID().toString()) }
    var submitting by remember(station.stationId) { mutableStateOf(false) }
    var result by rememberSaveable(station.stationId) { mutableStateOf<CorrectionSubmission?>(null) }
    val scope = rememberCoroutineScope()
    val fields = if (station.municipalityCode != null && station.state != null) CorrectionField.entries
        else CorrectionField.entries.filterNot { it == CorrectionField.Address }
    val sent = result == CorrectionSubmission.Sent

    AlertDialog(
        onDismissRequest = { if (!submitting) onDismiss() },
        title = { Text(stringResource(R.string.station_correction_title)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                if (sent) {
                    Text(stringResource(R.string.station_correction_sent))
                } else {
                    Text(stringResource(R.string.station_correction_review))
                    fields.forEach { option ->
                        FilterChip(
                            selected = field == option,
                            enabled = !submitting,
                            onClick = {
                                if (field != option) {
                                    field = option
                                    value = ""
                                    key = UUID.randomUUID().toString()
                                    result = null
                                }
                            },
                            label = { Text(stringResource(option.label)) },
                        )
                    }
                    if (field != CorrectionField.Closed) {
                        OutlinedTextField(
                            value = value,
                            onValueChange = {
                                value = it
                                key = UUID.randomUUID().toString()
                                result = null
                            },
                            enabled = !submitting,
                            label = { Text(stringResource(field.label)) },
                            singleLine = true,
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                    if (result == CorrectionSubmission.Failed) {
                        Text(stringResource(R.string.station_correction_failed), color = MaterialTheme.colorScheme.error)
                    }
                }
            }
        },
        confirmButton = {
            if (sent) {
                TextButton(onClick = onDismiss) { Text(stringResource(R.string.feed_close)) }
            } else {
                TextButton(
                    enabled = !submitting && (field == CorrectionField.Closed || value.trim().length in 1..160),
                    onClick = {
                        val payload = when (field) {
                            CorrectionField.Name -> JSONObject().put("value", value.trim())
                            CorrectionField.Address -> JSONObject().put("street", value.trim())
                                .put("municipality_code", station.municipalityCode).put("state", station.state)
                            CorrectionField.Closed -> JSONObject().put("status", "permanently_closed")
                        }
                        submitting = true
                        scope.launch {
                            try {
                                result = viewModel.submit(station.stationId, key, field.wire, payload)
                                if (result == CorrectionSubmission.SignIn) onSignIn()
                            } finally {
                                submitting = false
                            }
                        }
                    },
                ) { Text(stringResource(R.string.suggest_submit)) }
            }
        },
        dismissButton = {
            if (!sent) {
                TextButton(onClick = onDismiss, enabled = !submitting) { Text(stringResource(R.string.action_cancel)) }
            }
        },
    )
}
