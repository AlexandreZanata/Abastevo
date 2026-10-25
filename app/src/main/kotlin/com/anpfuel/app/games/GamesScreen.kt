package com.anpfuel.app.games

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.ArrowDownward
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.EmojiEvents
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.SportsEsports
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Icon
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.anpfuel.app.R
import com.anpfuel.app.ui.components.AnpScaffold
import com.anpfuel.app.ui.components.AnpTopAppBar
import com.anpfuel.app.ui.theme.AnpContainerDefaults
import com.anpfuel.domain.games.ArcadeSession
import com.anpfuel.domain.games.GameAction
import com.anpfuel.domain.games.GameId
import com.anpfuel.domain.games.GamePhase
import com.anpfuel.domain.games.RunEnd
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private enum class SaveState { NONE, SAVING, SAVED, FAILED }

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GamesScreen(onNavigateBack: () -> Unit, modifier: Modifier = Modifier, preferences: GamesPreferences? = null) {
    val context = LocalContext.current
    val store = remember(context.applicationContext, preferences) { preferences ?: GamesPreferences(context) }
    val scope = rememberCoroutineScope()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    // Deliberately not saveable: recreation never reconstructs an unfinished run.
    var selected by remember { mutableStateOf<GameId?>(null) }
    var bests by remember { mutableStateOf<Map<GameId, Int>>(emptyMap()) }
    var options by remember { mutableStateOf(GameOptions()) }
    var loaded by remember { mutableStateOf(false) }
    val session = remember(selected) { selected?.let { ArcadeSession(it, System.nanoTime().toInt()) } }
    val view = remember(session, context) { session?.let { ArcadeView(context, it) } }
    var phase by remember(session) { mutableStateOf(session?.phase ?: GamePhase.READY) }
    var saveState by remember(session) { mutableStateOf(SaveState.NONE) }

    LaunchedEffect(store) {
        val saved = withContext(Dispatchers.IO) {
            store.options() to GameId.entries.associateWith { store.records.best(it) }
        }
        options = saved.first; bests = saved.second; loaded = true
    }

    fun saveFinished(game: ArcadeSession) {
        val gameId = game.gameId
        val completedScore = game.simulation.score
        saveState = SaveState.SAVING
        scope.launch(start = CoroutineStart.UNDISPATCHED) {
            // Navigating away may discard UI state, but an acknowledged completion
            // must finish its tiny disk commit while this process remains alive.
            val saved = withContext(NonCancellable + Dispatchers.IO) {
                store.records.record(gameId, GamePhase.FINISHED, completedScore)
            }
            bests = bests + (gameId to saved.best)
            saveState = if (saved.saved) SaveState.SAVED else SaveState.FAILED
        }
    }

    SideEffect {
        view?.muted = options.muted
        view?.reducedMotion = options.reducedMotion
        view?.lowPower = options.lowPower
        view?.onPhaseChanged = { next ->
            phase = next
            if (next == GamePhase.FINISHED && session != null) saveFinished(session)
        }
    }
    DisposableEffect(view, lifecycle) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_PAUSE || event == Lifecycle.Event.ON_STOP) view?.pause()
        }
        lifecycle.addObserver(observer)
        if (!lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)) view?.pause()
        onDispose { lifecycle.removeObserver(observer); view?.dispose() }
    }

    val back: () -> Unit = {
        if (phase == GamePhase.RUNNING) view?.pause()
        else if (selected != null) selected = null
        else onNavigateBack()
    }
    BackHandler(enabled = selected != null, onBack = back)
    AnpScaffold(
        modifier = modifier.fillMaxSize(),
        topBar = { AnpTopAppBar(title = { Text(if (selected == null) stringResource(R.string.games_title) else gameTitle(selected!!)) }, onNavigateUp = back) },
    ) { padding ->
        if (session == null || view == null) {
            GamesSelector(
                bests = bests, loaded = loaded, options = options,
                onSelect = { selected = it },
                onOptions = { options = it; store.saveOptions(it) },
                modifier = Modifier.padding(padding),
            )
        } else {
            BoxWithConstraints(Modifier.fillMaxSize().padding(padding).padding(8.dp)) {
                val stage: @Composable () -> Unit = {
                    Box(Modifier.fillMaxSize().clip(RoundedCornerShape(20.dp))) {
                        AndroidView(
                            factory = { view },
                            update = {
                                val playing = phase == GamePhase.RUNNING
                                it.isFocusable = playing
                                it.isClickable = playing
                                it.importantForAccessibility = if (playing) android.view.View.IMPORTANT_FOR_ACCESSIBILITY_YES
                                    else android.view.View.IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS
                            },
                            modifier = Modifier.fillMaxSize().testTag("games_playfield"),
                        )
                        if (phase != GamePhase.RUNNING) {
                            GameOverlay(
                                session, phase, bests[session.gameId] ?: 0, saveState,
                                onStart = view::start, onResume = view::resumeGame,
                                onRestart = { saveState = SaveState.NONE; view.restart() },
                                onSelect = { selected = null }, onRetrySave = { saveFinished(session) },
                            )
                        }
                    }
                }
                if (maxWidth > maxHeight) {
                    Row(Modifier.fillMaxSize(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Box(Modifier.weight(1f).fillMaxHeight()) { stage() }
                        // Keep the native stage at the same composition position.
                        // Moving it between Row/Column on start detaches and pauses it.
                        if (phase == GamePhase.RUNNING) {
                            Column(Modifier.width(200.dp).fillMaxHeight().verticalScroll(rememberScrollState())) {
                                GameControls(session.gameId, phase, view::action, view::pause)
                            }
                        }
                    }
                } else {
                    Column(Modifier.fillMaxSize(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Box(Modifier.weight(1f).fillMaxWidth()) { stage() }
                        if (phase == GamePhase.RUNNING) {
                            Column(Modifier.fillMaxWidth().heightIn(max = 240.dp).verticalScroll(rememberScrollState())) {
                                GameControls(session.gameId, phase, view::action, view::pause)
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun GamesSelector(
    bests: Map<GameId, Int>, loaded: Boolean, options: GameOptions,
    onSelect: (GameId) -> Unit, onOptions: (GameOptions) -> Unit, modifier: Modifier,
) {
    Column(
        modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp),
        verticalArrangement = Arrangement.spacedBy(20.dp),
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.games_heading), style = MaterialTheme.typography.headlineMedium,
                    fontWeight = FontWeight.Bold, modifier = Modifier.semantics { heading() })
                Text(stringResource(R.string.games_subtitle), style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Icon(Icons.Default.SportsEsports, null, tint = MaterialTheme.colorScheme.primary,
                modifier = Modifier.size(52.dp).clip(RoundedCornerShape(16.dp))
                    .background(MaterialTheme.colorScheme.primaryContainer).padding(12.dp))
        }
        Text(stringResource(R.string.games_no_rank), style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.primary)
        BoxWithConstraints(Modifier.fillMaxWidth()) {
            if (maxWidth >= 640.dp) {
                Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                    for (game in GameId.entries) {
                        GameCard(game, bests[game] ?: 0, loaded, { onSelect(game) }, Modifier.weight(1f))
                    }
                }
            } else {
                Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
                    for (game in GameId.entries) GameCard(game, bests[game] ?: 0, loaded, { onSelect(game) })
                }
            }
        }
        val settingsContainer = MaterialTheme.colorScheme.surfaceContainerLow
        Card(shape = AnpContainerDefaults.containerShape(), colors = CardDefaults.cardColors(
            containerColor = settingsContainer), border = AnpContainerDefaults.borderStroke(settingsContainer)) {
            Column(Modifier.padding(16.dp)) {
                Text(stringResource(R.string.games_settings), style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.Bold, modifier = Modifier.semantics { heading() })
                if (loaded) {
                    GameOption(stringResource(R.string.games_muted), options.muted) { onOptions(options.copy(muted = it)) }
                    GameOption(stringResource(R.string.games_reduced_motion), options.reducedMotion) { onOptions(options.copy(reducedMotion = it)) }
                    GameOption(stringResource(R.string.games_low_power), options.lowPower) { onOptions(options.copy(lowPower = it)) }
                }
            }
        }
    }
}

@Composable
private fun GameCard(game: GameId, best: Int, loaded: Boolean, onChoose: () -> Unit, modifier: Modifier = Modifier) {
    val containerColor = MaterialTheme.colorScheme.surfaceContainerLow
    Card(modifier.fillMaxWidth(), shape = AnpContainerDefaults.containerShape(),
        border = AnpContainerDefaults.borderStroke(containerColor),
        colors = CardDefaults.cardColors(containerColor = containerColor)) {
        Box(Modifier.fillMaxWidth().aspectRatio(2.65f)) {
            GameArtwork(game, Modifier.fillMaxSize())
            Box(Modifier.fillMaxSize().background(Brush.verticalGradient(listOf(Color.Transparent, Color(0xCC0B1830)))))
            Text(gameTitle(game), color = Color.White, style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.Bold, modifier = Modifier.align(Alignment.BottomStart).padding(16.dp))
        }
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text(stringResource(if (game == GameId.FLIGHT) R.string.games_flight_summary else R.string.games_run_summary),
                style = MaterialTheme.typography.bodyMedium)
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Default.EmojiEvents, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.primary)
                Text(if (loaded) stringResource(R.string.games_local_best, best) else stringResource(R.string.games_loading),
                    style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Button(onChoose, enabled = loaded, shape = RoundedCornerShape(14.dp),
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("games_choose_${game.name}")) {
                Icon(Icons.Default.PlayArrow, null, Modifier.size(20.dp))
                Spacer(Modifier.width(6.dp))
                Text(stringResource(R.string.games_choose, gameTitle(game)))
            }
        }
    }
}

@Composable
private fun GameOption(label: String, checked: Boolean, onChecked: (Boolean) -> Unit) {
    Row(
        Modifier.fillMaxWidth().heightIn(min = 56.dp).toggleable(checked, role = Role.Switch, onValueChange = onChecked).padding(horizontal = 8.dp),
        verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(label, modifier = Modifier.weight(1f))
        Switch(checked = checked, onCheckedChange = null)
    }
}

@Composable
private fun GameOverlay(
    session: ArcadeSession, phase: GamePhase, best: Int, saveState: SaveState,
    onStart: () -> Unit, onResume: () -> Unit, onRestart: () -> Unit,
    onSelect: () -> Unit, onRetrySave: () -> Unit,
) {
    Box(Modifier.fillMaxSize().background(Color(0x990B1830)), contentAlignment = Alignment.Center) {
        Column(
            Modifier.padding(16.dp).widthIn(max = 480.dp).fillMaxWidth().clip(AnpContainerDefaults.containerShape())
                .background(MaterialTheme.colorScheme.surface).verticalScroll(rememberScrollState()).padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                Icon(when (phase) {
                    GamePhase.READY -> Icons.Default.SportsEsports
                    GamePhase.PAUSED -> Icons.Default.Pause
                    else -> Icons.Default.EmojiEvents
                }, null, tint = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.size(44.dp).clip(RoundedCornerShape(14.dp))
                        .background(MaterialTheme.colorScheme.primaryContainer).padding(10.dp))
                Text(stringResource(when (phase) {
                    GamePhase.READY -> R.string.games_how_to
                    GamePhase.PAUSED -> R.string.games_paused
                    else -> R.string.games_finish
                }), style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold,
                    modifier = Modifier.weight(1f).semantics { heading() })
            }
            when (phase) {
                GamePhase.READY -> {
                    Text(stringResource(if (session.gameId == GameId.FLIGHT) R.string.games_flight_tutorial else R.string.games_run_tutorial),
                        style = MaterialTheme.typography.bodyLarge)
                    Text(stringResource(R.string.games_limit_notice), style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Button(onStart, Modifier.fillMaxWidth().heightIn(min = 52.dp).testTag("games_start"), shape = RoundedCornerShape(14.dp)) {
                        Icon(Icons.Default.PlayArrow, null); Spacer(Modifier.width(6.dp)); Text(stringResource(R.string.games_start))
                    }
                }
                GamePhase.PAUSED -> {
                    Text(stringResource(R.string.games_pause_notice))
                    Button(onResume, Modifier.fillMaxWidth().heightIn(min = 52.dp).testTag("games_resume"), shape = RoundedCornerShape(14.dp)) {
                        Icon(Icons.Default.PlayArrow, null); Spacer(Modifier.width(6.dp)); Text(stringResource(R.string.games_continue))
                    }
                    OutlinedButton(onRestart, Modifier.fillMaxWidth().heightIn(min = 48.dp), shape = RoundedCornerShape(14.dp)) {
                        Icon(Icons.Default.Refresh, null); Spacer(Modifier.width(6.dp)); Text(stringResource(R.string.games_restart))
                    }
                }
                GamePhase.FINISHED -> {
                    Text(stringResource(if (session.simulation.end == RunEnd.TIME_LIMIT) R.string.games_time_limit else R.string.games_collision))
                    Text(stringResource(R.string.games_score, session.simulation.score), style = MaterialTheme.typography.headlineSmall,
                        color = MaterialTheme.colorScheme.primary, fontWeight = FontWeight.Bold, modifier = Modifier.testTag("games_final_score"))
                    Text(stringResource(R.string.games_local_best, best), style = MaterialTheme.typography.titleMedium)
                    when (saveState) {
                        SaveState.SAVING -> Text(stringResource(R.string.games_saving))
                        SaveState.FAILED -> {
                            Text(stringResource(R.string.games_save_failed))
                            OutlinedButton(onRetrySave, Modifier.fillMaxWidth()) { Text(stringResource(R.string.games_retry_save)) }
                        }
                        else -> Unit
                    }
                    Button(onRestart, Modifier.fillMaxWidth().heightIn(min = 52.dp).testTag("games_restart"),
                        shape = RoundedCornerShape(14.dp), enabled = saveState != SaveState.SAVING) {
                        Icon(Icons.Default.Refresh, null); Spacer(Modifier.width(6.dp)); Text(stringResource(R.string.games_restart))
                    }
                }
                else -> Unit
            }
            TextButton(onSelect, Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text(stringResource(R.string.games_select_other)) }
        }
    }
}

@Composable
private fun GameControls(game: GameId, phase: GamePhase, onAction: (GameAction) -> Unit, onPause: () -> Unit) {
    val running = phase == GamePhase.RUNNING
    BoxWithConstraints(Modifier.fillMaxWidth()) {
        val narrow = maxWidth < 280.dp
        Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (game == GameId.FLIGHT) {
                ArcadeButton(R.string.games_fly, Icons.Default.ArrowUpward, GameAction.IMPULSE, running, onAction, Modifier.fillMaxWidth())
            } else if (narrow) {
                ArcadeButton(R.string.games_left, Icons.AutoMirrored.Filled.ArrowBack, GameAction.LEFT, running, onAction, Modifier.fillMaxWidth())
                ArcadeButton(R.string.games_right, Icons.AutoMirrored.Filled.ArrowForward, GameAction.RIGHT, running, onAction, Modifier.fillMaxWidth())
                ArcadeButton(R.string.games_jump, Icons.Default.ArrowUpward, GameAction.JUMP, running, onAction, Modifier.fillMaxWidth())
                ArcadeButton(R.string.games_slide, Icons.Default.ArrowDownward, GameAction.SLIDE, running, onAction, Modifier.fillMaxWidth())
            } else {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    ArcadeButton(R.string.games_left, Icons.AutoMirrored.Filled.ArrowBack, GameAction.LEFT, running, onAction, Modifier.weight(1f))
                    ArcadeButton(R.string.games_right, Icons.AutoMirrored.Filled.ArrowForward, GameAction.RIGHT, running, onAction, Modifier.weight(1f))
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    ArcadeButton(R.string.games_jump, Icons.Default.ArrowUpward, GameAction.JUMP, running, onAction, Modifier.weight(1f))
                    ArcadeButton(R.string.games_slide, Icons.Default.ArrowDownward, GameAction.SLIDE, running, onAction, Modifier.weight(1f))
                }
            }
            OutlinedButton(onPause, Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("games_pause"),
                shape = RoundedCornerShape(14.dp), enabled = running) {
                Icon(Icons.Default.Pause, null, Modifier.size(20.dp)); Spacer(Modifier.width(6.dp)); Text(stringResource(R.string.games_pause))
            }
        }
    }
}

@Composable
private fun ArcadeButton(label: Int, icon: ImageVector, action: GameAction, enabled: Boolean,
    onAction: (GameAction) -> Unit, modifier: Modifier) {
    Button({ onAction(action) }, modifier.heightIn(min = 52.dp).testTag("games_action_${action.name}"),
        shape = RoundedCornerShape(14.dp), enabled = enabled,
        colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.primaryContainer,
            contentColor = MaterialTheme.colorScheme.onPrimaryContainer)) {
        Icon(icon, null, Modifier.size(22.dp))
        Spacer(Modifier.width(6.dp))
        Text(stringResource(label), textAlign = TextAlign.Center, modifier = Modifier.weight(1f))
    }
}

@Composable
private fun gameTitle(game: GameId): String = stringResource(if (game == GameId.FLIGHT) R.string.games_flight_title else R.string.games_run_title)
