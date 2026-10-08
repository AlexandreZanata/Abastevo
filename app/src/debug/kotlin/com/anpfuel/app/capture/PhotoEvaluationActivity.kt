package com.anpfuel.app.capture

import android.content.Context
import com.anpfuel.app.locale.AppLocaleApplier
import com.anpfuel.app.locale.AppLocaleHolder
import android.Manifest
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.data.local.ocr.MlKitImagePriceOcr
import com.anpfuel.domain.valueobject.FuelProduct
import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** Owned historical corpus and controlled proximity, local only: no proof transport or outbox. */
class PhotoEvaluationActivity : ComponentActivity() {
    private val media by lazy { PrivateCaptureFiles(this) }
    private val ocr by lazy { MlKitImagePriceOcr(this) }
    private var photoUri by mutableStateOf<Uri?>(null)
    private var rows by mutableStateOf<Map<FuelProduct, String>>(emptyMap())
    private var removed by mutableStateOf<Set<FuelProduct>>(emptySet())
    private var orphans by mutableStateOf<List<String>>(emptyList())
    private val edited = mutableSetOf<FuelProduct>()
    var processing by mutableStateOf(false)
        private set
    var conditional by mutableStateOf(false)
        private set
    var queuedLocally by mutableStateOf(false)
        private set
    var reviewedCount = 0
        private set
    val visibleRowCount get() = rows.size
    private var capturedAt = 0L
    private var pendingUri: Uri? = null
    private var generation = 0
    private var message by mutableStateOf("")
    private var simulatedDistance by mutableStateOf(150.0)

    override fun attachBaseContext(newBase: Context) {
        super.attachBaseContext(AppLocaleApplier.wrap(newBase, AppLocaleHolder.localeTag))
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        savedInstanceState?.let { restored ->
            photoUri = restored.getString("reviewUri")?.let(Uri::parse)
            pendingUri = restored.getString("pendingUri")?.let(Uri::parse)
            capturedAt = restored.getLong("capturedAt")
            rows = restored.getStringArrayList("rows").orEmpty().mapNotNull { value ->
                val parts = value.split('=', limit = 2)
                runCatching { FuelProduct.valueOf(parts[0]) to parts[1] }.getOrNull()
            }.toMap()
            removed = restored.getStringArrayList("removed").orEmpty().mapNotNull { runCatching { FuelProduct.valueOf(it) }.getOrNull() }.toSet()
            edited += restored.getStringArrayList("edited").orEmpty().mapNotNull { runCatching { FuelProduct.valueOf(it) }.getOrNull() }
            conditional = restored.getBoolean("conditional")
            simulatedDistance = restored.getDouble("distance", 150.0)
        }
        setContent {
            AnpFuelTheme(darkTheme = false, dynamicColor = false) {
                var approved by remember { mutableStateOf(false) }
                val takePicture = rememberLauncherForActivityResult(ActivityResultContracts.TakePicture()) { success ->
                    val uri = pendingUri
                    pendingUri = null
                    if (success && uri != null) lifecycleScope.launch {
                        val bytes = withContext(Dispatchers.IO) { media.read(uri, capturedAt) }
                        if (bytes != null) { photoUri = uri; recognize(bytes) }
                        else message = "Foto inválida"
                    } else message = "Câmera cancelada; nenhum envio"
                }
                val openCamera: () -> Unit = {
                    lifecycleScope.launch {
                        capturedAt = System.currentTimeMillis()
                        val uri = withContext(Dispatchers.IO) { media.create(capturedAt) }
                        pendingUri = uri
                        takePicture.launch(uri)
                    }
                }
                val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
                    if (granted) openCamera() else message = "Permissão de câmera negada"
                }
                Surface {
                    Column(Modifier.fillMaxSize().safeDrawingPadding().verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        Text("Teste local · localização simulada · nenhum envio", style = MaterialTheme.typography.titleMedium)
                        Text("Distância simulada: ${simulatedDistance.toInt()} m")
                        Row {
                            TextButton(onClick = { simulatedDistance = 150.0 }) { Text("Dentro: 150 m") }
                            TextButton(onClick = { simulatedDistance = 151.0 }) { Text("Fora: 151 m") }
                        }
                        Button(onClick = {
                            if (!CaptureOcrViewModel.isInsideStationArea(simulatedDistance)) {
                                message = "Fique a até 150 metros do posto. Câmera não aberta."
                            } else if (ContextCompat.checkSelfPermission(this@PhotoEvaluationActivity, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) openCamera()
                            else permission.launch(Manifest.permission.CAMERA)
                        }) { Text("Contribuir com foto (teste)") }
                        if (message.isNotEmpty()) Text(message)
                        photoUri?.let { uri ->
                            PhotoReviewContent(
                                photoUri = uri, fuelAmounts = rows, removedFuels = removed,
                                fuelErrors = emptySet(), unassigned = orphans,
                                photoAttached = true, photoRefused = null, submit = null,
                                onAmount = { product, value -> edited += product; rows = rows + (product to value) },
                                onRemoveFuel = { product -> edited += product; removed = removed + product; rows = rows - product },
                                onAddFuel = { product -> edited += product; removed = removed - product; rows = rows + (product to "") },
                                onAssignUnassigned = { index, product ->
                                    orphans.getOrNull(index)?.let { amount ->
                                        edited += product; rows = rows + (product to amount)
                                        orphans = orphans.filterIndexed { i, _ -> i != index }
                                    }
                                },
                                onDismissUnassigned = { index -> orphans = orphans.filterIndexed { i, _ -> i != index } },
                                processing = processing, conditional = conditional, defaultPriceConfirmed = approved,
                                onConfirmDefault = { approved = it },
                                onReanalyze = { lifecycleScope.launch { withContext(Dispatchers.IO) { media.read(uri, capturedAt) }?.let { recognize(it) } } },
                                onCrop = { bytes -> lifecycleScope.launch {
                                    val next = withContext(Dispatchers.IO) { media.write(bytes, capturedAt) }
                                    photoUri = next
                                    withContext(Dispatchers.IO) { media.delete(uri) }
                                    recognize(bytes)
                                } },
                                onRetake = openCamera,
                                onSubmit = {
                                    reviewedCount = rows.count { (product, value) -> product !in removed && value.isNotBlank() }
                                    queuedLocally = true
                                    message = "${reviewedCount} preços revisados no teste. Nenhum preço ou foto foi enviado."
                                },
                            )
                        }
                    }
                }
            }
        }
        if (savedInstanceState == null) intent.getStringExtra("image")?.let(::loadEvaluationImage)
    }

    override fun onSaveInstanceState(outState: Bundle) {
        outState.putString("reviewUri", photoUri?.toString())
        outState.putString("pendingUri", pendingUri?.toString())
        outState.putLong("capturedAt", capturedAt)
        outState.putStringArrayList("rows", ArrayList(rows.map { "${it.key.name}=${it.value}" }))
        outState.putStringArrayList("removed", ArrayList(removed.map { it.name }))
        outState.putStringArrayList("edited", ArrayList(edited.map { it.name }))
        outState.putBoolean("conditional", conditional)
        outState.putDouble("distance", simulatedDistance)
        super.onSaveInstanceState(outState)
    }

    fun loadEvaluationImage(name: String) {
        if (!name.matches(Regex("ocr-[0-9]{4}\\.jpeg"))) {
            message = "Imagem de avaliação inválida"
            return
        }
        lifecycleScope.launch {
            val file = File(filesDir, "ocr-evaluation/$name")
            val bytes = withContext(Dispatchers.IO) {
                if (!file.isFile || file.length() !in 1..PrivateCaptureFiles.MAX_BYTES.toLong()) {
                    // Missing evaluation asset (e.g. cleaned corpus) must
                    // never crash app open when the task is recreated.
                    null
                } else file.readBytes()
            } ?: run {
                message = "Imagem de avaliação ausente"
                return@launch
            }
            capturedAt = System.currentTimeMillis() // Local evaluation age; never an observation timestamp.
            val uri = withContext(Dispatchers.IO) { media.write(bytes, capturedAt) }
            photoUri?.let { old -> withContext(Dispatchers.IO) { media.delete(old) } }
            photoUri = uri
            rows = emptyMap(); removed = emptySet(); edited.clear(); conditional = false; queuedLocally = false
            recognize(bytes)
        }
    }

    private fun recognize(bytes: ByteArray) {
        val ticket = ++generation
        processing = true
        lifecycleScope.launch {
            try {
                val result = ocr.recognize(bytes)
                if (ticket != generation) return@launch
                conditional = conditional || result.conditional
                rows = rows.filterKeys { it in edited } + result.rows.filter { it.product !in edited && it.product !in removed }
                    .associate { it.product to "${it.amountMilli / 1000},${(it.amountMilli % 1000).toString().padStart(3, '0')}" }
                val assigned = rows.values.toSet()
                orphans = result.orphans
                    .map { "${it / 1000},${(it % 1000).toString().padStart(3, '0')}" }
                    .filter { it !in assigned }.distinct()
            } catch (_: Exception) { if (ticket == generation) message = "Leitura indisponível. Recorte ou digite os preços." }
            finally { if (ticket == generation) processing = false }
        }
    }
}
