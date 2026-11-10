package com.anpfuel.app.capture

import android.annotation.SuppressLint
import android.net.Uri
import android.os.SystemClock
import android.util.Size
import android.view.MotionEvent
import androidx.camera.core.CameraSelector
import androidx.camera.core.ExperimentalGetImage
import androidx.camera.core.ImageCapture
import androidx.camera.core.ImageCaptureException
import androidx.camera.core.resolutionselector.AspectRatioStrategy
import androidx.camera.core.resolutionselector.ResolutionSelector
import androidx.camera.core.resolutionselector.ResolutionStrategy
import androidx.camera.view.CameraController
import androidx.camera.view.LifecycleCameraController
import androidx.camera.view.PreviewView
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size as GeometrySize
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.compose.ui.window.DialogWindowProvider
import androidx.core.content.ContextCompat
import androidx.core.view.WindowCompat
import androidx.lifecycle.Observer
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AbastevoSpacing
import com.anpfuel.app.ui.theme.DarkColorScheme
import com.anpfuel.data.local.ocr.MlKitLiveBoardLocator
import com.anpfuel.domain.portable.LiveBoardZoom
import java.io.OutputStream
import java.util.Locale
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong
import kotlin.math.pow
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** Frames read while the zoom changes are discarded until this long after it settles. */
private const val SETTLE_MILLIS = 150L
/** Without a streaming preview by then, the system camera takes over. */
private const val START_TIMEOUT_MILLIS = 5000L
private const val ZOOM_STEPS = 8
private const val ZOOM_STEP_MILLIS = 30L

/**
 * In-app camera for the price photo (B-BR-PC02 live framing): preview frames find the board,
 * [LiveBoardZoom] zooms onto it and a frame shows where it is. Only the shutter takes the
 * photo, written to the private [photo] file; [onDone] reports whether it was written.
 * [onUnavailable] hands over to the system camera when CameraX cannot start.
 */
@SuppressLint("ClickableViewAccessibility")
// The analyzer hands the frame's own Image to the recognizer and closes it when the reading ends.
@androidx.annotation.OptIn(markerClass = [ExperimentalGetImage::class])
@Composable
internal fun LiveBoardCamera(photo: Uri, onDone: (Boolean) -> Unit, onUnavailable: () -> Unit) {
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    val scope = rememberCoroutineScope()
    val done by rememberUpdatedState(onDone)
    val unavailable by rememberUpdatedState(onUnavailable)
    var board by remember { mutableStateOf<LiveBoardZoom.Board?>(null) }
    var zoom by remember { mutableStateOf<LiveBoardZoom.State?>(null) }
    var shownRatio by remember { mutableStateOf(1f) }
    var manual by remember { mutableStateOf(false) }
    var streaming by remember { mutableStateOf(false) }
    var capturing by remember { mutableStateOf(false) }
    var failed by remember { mutableStateOf(false) }
    val busy = remember { AtomicBoolean(false) }
    // Exactly one outcome per camera session: a photo, a cancel or the hand-over to the system camera.
    val finished = remember { AtomicBoolean(false) }
    fun finish(written: Boolean) { if (finished.compareAndSet(false, true)) done(written) }
    fun handOver() { if (finished.compareAndSet(false, true)) unavailable() }
    val settledAt = remember { AtomicLong(0L) }
    var zoomJob by remember { mutableStateOf<Job?>(null) }
    val controller = remember {
        val fourByThree = AspectRatioStrategy.RATIO_4_3_FALLBACK_AUTO_STRATEGY
        LifecycleCameraController(context).apply {
            cameraSelector = CameraSelector.DEFAULT_BACK_CAMERA
            setEnabledUseCases(CameraController.IMAGE_CAPTURE or CameraController.IMAGE_ANALYSIS)
            imageCaptureMode = ImageCapture.CAPTURE_MODE_MAXIMIZE_QUALITY
            previewResolutionSelector = ResolutionSelector.Builder().setAspectRatioStrategy(fourByThree).build()
            // The camera's largest 4:3 photo: the post-photo reading cuts the board from these pixels.
            imageCaptureResolutionSelector = ResolutionSelector.Builder().setAspectRatioStrategy(fourByThree)
                .setResolutionStrategy(ResolutionStrategy.HIGHEST_AVAILABLE_STRATEGY).build()
            // Large enough to read a board a few metres away, small enough to read a few times a second.
            imageAnalysisResolutionSelector = ResolutionSelector.Builder().setAspectRatioStrategy(fourByThree)
                .setResolutionStrategy(ResolutionStrategy(Size(1280, 960), ResolutionStrategy.FALLBACK_RULE_CLOSEST_LOWER_THEN_HIGHER))
                .build()
        }
    }
    val previewView = remember {
        PreviewView(context).apply {
            implementationMode = PreviewView.ImplementationMode.COMPATIBLE
            scaleType = PreviewView.ScaleType.FILL_CENTER
            this.controller = controller
            // A pinch is the contributor's own zoom: automatic zoom stops for this session.
            setOnTouchListener { _, event ->
                if (event.pointerCount > 1 && event.actionMasked == MotionEvent.ACTION_POINTER_DOWN) {
                    manual = true
                    zoomJob?.cancel()
                }
                false
            }
        }
    }

    fun onBoard(found: LiveBoardZoom.Board?, readAt: Long) {
        if (finished.get() || capturing || readAt < settledAt.get()) return
        board = found
        val current = zoom ?: return
        if (manual) return
        val range = controller.zoomState.value ?: return
        val next = LiveBoardZoom.next(current, found, range.minZoomRatio.toDouble(), range.maxZoomRatio.toDouble())
        zoom = next
        if (next.ratio == current.ratio) return
        // The frame moves with the picture while the zoom glides.
        board = found?.zoomedBy(next.ratio / current.ratio)
        zoomJob?.cancel()
        settledAt.set(Long.MAX_VALUE)
        zoomJob = scope.launch {
            try {
                // Geometric steps look like an even glide at every zoom level.
                for (step in 1..ZOOM_STEPS) {
                    val ratio = current.ratio * (next.ratio / current.ratio).pow(step.toDouble() / ZOOM_STEPS)
                    controller.setZoomRatio(ratio.toFloat())
                    delay(ZOOM_STEP_MILLIS)
                }
            } finally {
                settledAt.set(SystemClock.elapsedRealtime() + SETTLE_MILLIS)
            }
        }
    }

    DisposableEffect(lifecycleOwner) {
        val locator = MlKitLiveBoardLocator.open()
        val analysis = Executors.newSingleThreadExecutor()
        val main = ContextCompat.getMainExecutor(context)
        controller.setImageAnalysisAnalyzer(analysis) { proxy ->
            val image = proxy.image
            if (image == null || busy.get()) { proxy.close(); return@setImageAnalysisAnalyzer }
            val readAt = SystemClock.elapsedRealtime()
            // The analyzer keeps only the latest frame: until this one is closed, newer ones are dropped.
            locator.locate(image, proxy.imageInfo.rotationDegrees) { found ->
                proxy.close()
                main.execute { onBoard(found, readAt) }
            }
        }
        val zoomObserver = Observer<androidx.camera.core.ZoomState> { state ->
            shownRatio = state.zoomRatio
            if (zoom == null) zoom = LiveBoardZoom.State(state.zoomRatio.toDouble())
        }
        val streamObserver = Observer<PreviewView.StreamState> { streaming = it == PreviewView.StreamState.STREAMING }
        controller.zoomState.observe(lifecycleOwner, zoomObserver)
        previewView.previewStreamState.observe(lifecycleOwner, streamObserver)
        try {
            controller.bindToLifecycle(lifecycleOwner)
        } catch (_: Exception) {
            main.execute { handOver() }
        }
        controller.initializationFuture.addListener({
            val ready = runCatching { controller.initializationFuture.get(); controller.hasCamera(CameraSelector.DEFAULT_BACK_CAMERA) }
            if (ready.getOrDefault(false) != true) handOver()
        }, main)
        onDispose {
            finished.set(true)
            busy.set(true)
            zoomJob?.cancel()
            controller.zoomState.removeObserver(zoomObserver)
            previewView.previewStreamState.removeObserver(streamObserver)
            controller.clearImageAnalysisAnalyzer()
            controller.unbind()
            analysis.shutdown()
            locator.close()
        }
    }
    LaunchedEffect(Unit) {
        delay(START_TIMEOUT_MILLIS)
        if (!streaming) handOver()
    }

    fun shoot() {
        if (capturing || !streaming) return
        capturing = true
        failed = false
        busy.set(true)
        zoomJob?.cancel()
        val stream: OutputStream? = runCatching { context.contentResolver.openOutputStream(photo) }.getOrNull()
        if (stream == null) { capturing = false; busy.set(false); failed = true; return }
        controller.takePicture(
            ImageCapture.OutputFileOptions.Builder(stream).build(),
            ContextCompat.getMainExecutor(context),
            object : ImageCapture.OnImageSavedCallback {
                override fun onImageSaved(output: ImageCapture.OutputFileResults) {
                    runCatching { stream.close() }
                    finish(true)
                }

                override fun onError(exception: ImageCaptureException) {
                    runCatching { stream.close() }
                    capturing = false
                    busy.set(false)
                    failed = true
                }
            },
        )
    }

    // The dialog window is not given the system bar insets: measure them in the activity's window.
    val bars = WindowInsets.systemBars.asPaddingValues()
    // Its own full-screen window, above the app's bottom bar; back closes it like the close button.
    Dialog(
        onDismissRequest = { if (!capturing) finish(false) },
        properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false, dismissOnClickOutside = false),
    ) {
        CameraSystemBars()
        // The viewfinder is always dark, whatever the app theme.
        MaterialTheme(colorScheme = DarkColorScheme, typography = MaterialTheme.typography, shapes = MaterialTheme.shapes) {
            val colors = MaterialTheme.colorScheme
            Box(Modifier.fillMaxSize().background(colors.background)) {
                Box(Modifier.align(Alignment.Center).fillMaxWidth().aspectRatio(3f / 4f)) {
                    AndroidView(factory = { previewView }, modifier = Modifier.fillMaxSize())
                    BoardOverlay(if (streaming) board else null, Modifier.fillMaxSize())
                }
                Row(
                    Modifier.align(Alignment.TopCenter).fillMaxWidth().padding(top = bars.calculateTopPadding())
                        .padding(horizontal = AbastevoSpacing.screenGutter, vertical = AbastevoSpacing.sm),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    IconButton(
                        onClick = { finish(false) },
                        enabled = !capturing,
                        colors = IconButtonDefaults.iconButtonColors(containerColor = colors.surfaceContainerHigh, contentColor = colors.onSurface),
                    ) { Icon(Icons.Default.Close, contentDescription = stringResource(R.string.capture_live_close)) }
                    Spacer(Modifier.weight(1f))
                    CameraPill(
                        text = String.format(Locale.getDefault(), "%.1fx", shownRatio).let {
                            if (manual) stringResource(R.string.capture_live_zoom_manual, it) else it
                        },
                        accent = false,
                    )
                }
                Column(
                    Modifier.align(Alignment.BottomCenter).fillMaxWidth().padding(bottom = bars.calculateBottomPadding() + AbastevoSpacing.xl),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(AbastevoSpacing.lg),
                ) {
                    val found = streaming && board != null
                    CameraPill(
                        text = stringResource(
                            when {
                                failed -> R.string.capture_live_failed
                                found -> R.string.capture_live_found
                                else -> R.string.capture_live_point
                            },
                        ),
                        accent = found && !failed,
                    )
                    Shutter(enabled = streaming && !capturing, capturing = capturing, onClick = ::shoot)
                }
            }
        }
    }
}

/** Where the board appears after the centre crop grows by [factor]; null once it leaves the frame. */
private fun LiveBoardZoom.Board.zoomedBy(factor: Double): LiveBoardZoom.Board? {
    fun scale(value: Double) = (0.5 + (value - 0.5) * factor).coerceIn(0.0, 1.0)
    val l = scale(left); val t = scale(top); val r = scale(right); val b = scale(bottom)
    return if (l < r && t < b) LiveBoardZoom.Board(l, t, r, b) else null
}

@Composable
private fun CameraPill(text: String, accent: Boolean) {
    val colors = MaterialTheme.colorScheme
    Surface(shape = CircleShape, color = if (accent) colors.secondaryContainer else colors.surfaceContainerHigh) {
        Text(
            text,
            modifier = Modifier.padding(horizontal = AbastevoSpacing.lg, vertical = AbastevoSpacing.sm),
            style = MaterialTheme.typography.labelLarge,
            fontWeight = FontWeight.SemiBold,
            color = if (accent) colors.onSecondaryContainer else colors.onSurface,
        )
    }
}

@Composable
private fun Shutter(enabled: Boolean, capturing: Boolean, onClick: () -> Unit) {
    val ink = MaterialTheme.colorScheme.onSurface
    val description = stringResource(R.string.capture_live_shutter)
    Box(
        Modifier.size(76.dp).border(4.dp, ink.copy(alpha = if (enabled) 1f else 0.4f), CircleShape).padding(8.dp)
            .background(ink.copy(alpha = if (enabled) 1f else 0.4f), CircleShape)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .semantics { contentDescription = description },
        contentAlignment = Alignment.Center,
    ) {
        if (capturing) CircularProgressIndicator(Modifier.size(28.dp), color = MaterialTheme.colorScheme.background, strokeWidth = 3.dp)
    }
}

/** Accent frame around the board when one is found, neutral corner guides otherwise. */
@Composable
private fun BoardOverlay(board: LiveBoardZoom.Board?, modifier: Modifier) {
    val accent = MaterialTheme.colorScheme.secondary
    val guide = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.7f)
    val left by animateFloatAsState((board?.left ?: 0.1).toFloat(), label = "boardLeft")
    val top by animateFloatAsState((board?.top ?: 0.3).toFloat(), label = "boardTop")
    val right by animateFloatAsState((board?.right ?: 0.9).toFloat(), label = "boardRight")
    val bottom by animateFloatAsState((board?.bottom ?: 0.7).toFloat(), label = "boardBottom")
    Canvas(modifier) {
        val origin = Offset(left * size.width, top * size.height)
        val extent = GeometrySize((right - left) * size.width, (bottom - top) * size.height)
        val width = 3.dp.toPx()
        if (board != null) {
            drawRoundRect(accent, origin, extent, CornerRadius(12.dp.toPx()), style = Stroke(width))
        } else {
            val arm = 28.dp.toPx()
            val corners = listOf(
                origin to Offset(1f, 1f),
                Offset(origin.x + extent.width, origin.y) to Offset(-1f, 1f),
                Offset(origin.x, origin.y + extent.height) to Offset(1f, -1f),
                Offset(origin.x + extent.width, origin.y + extent.height) to Offset(-1f, -1f),
            )
            corners.forEach { (corner, direction) ->
                drawLine(guide, corner, Offset(corner.x + arm * direction.x, corner.y), width)
                drawLine(guide, corner, Offset(corner.x, corner.y + arm * direction.y), width)
            }
        }
    }
}

/** The camera window draws edge to edge, undimmed, with light system bar icons over the viewfinder. */
@Composable
private fun CameraSystemBars() {
    val view = LocalView.current
    DisposableEffect(view) {
        (view.parent as? DialogWindowProvider)?.window?.let { window ->
            // Placed against the whole screen, not below the status bar, so the insets below are real.
            window.addFlags(android.view.WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN)
            window.setDimAmount(0f)
            @Suppress("DEPRECATION")
            window.statusBarColor = android.graphics.Color.TRANSPARENT
            @Suppress("DEPRECATION")
            window.navigationBarColor = android.graphics.Color.TRANSPARENT
            if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.P) {
                window.attributes = window.attributes.apply {
                    layoutInDisplayCutoutMode = android.view.WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES
                }
            }
            WindowCompat.getInsetsController(window, view).apply {
                isAppearanceLightStatusBars = false
                isAppearanceLightNavigationBars = false
            }
        }
        onDispose {}
    }
}
