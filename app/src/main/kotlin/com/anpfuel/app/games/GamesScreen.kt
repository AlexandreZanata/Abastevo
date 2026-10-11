package com.anpfuel.app.games

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
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
                        Column(Modifier.width(200.dp).fillMaxHeight().verticalScroll(rememberScrollState())) {
                            GameControls(session.gameId, phase, view::action, view::pause)
                        }
                    }
                } else {
                    Column(Modifier.fillMaxSize(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Box(Modifier.weight(1f).fillMaxWidth()) { stage() }
                        Column(Modifier.fillMaxWidth().heightIn(max = 240.dp).verticalScroll(rememberScrollState())) {
                            GameControls(session.gameId, phase, view::action, view::pause)
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
        modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text(stringResource(R.string.games_heading), style = MaterialTheme.typography.headlineSmall, modifier = Modifier.semantics { heading() })
        Text(stringResource(R.string.games_subtitle))
        for (game in GameId.entries) {
            Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.secondaryContainer), modifier = Modifier.fillMaxWidth()) {
                Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text(gameTitle(game), style = MaterialTheme.typography.titleLarge)
                    Text(stringResource(if (game == GameId.FLIGHT) R.string.games_flight_summary else R.string.games_run_summary))
                    Text(if (loaded) stringResource(R.string.games_local_best, bests[game] ?: 0) else stringResource(R.string.games_loading))
                    Button(onClick = { onSelect(game) }, enabled = loaded, modifier = Modifier.fillMaxWidth().testTag("games_choose_${game.name}")) {
                        Text(stringResource(R.string.games_choose, gameTitle(game)))
                    }
                }
            }
        }
        Text(stringResource(R.string.games_settings), style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
        if (loaded) {
            GameOption(stringResource(R.string.games_muted), options.muted) { onOptions(options.copy(muted = it)) }
            GameOption(stringResource(R.string.games_reduced_motion), options.reducedMotion) { onOptions(options.copy(reducedMotion = it)) }
            GameOption(stringResource(R.string.games_low_power), options.lowPower) { onOptions(options.copy(lowPower = it)) }
        }
        Text(stringResource(R.string.games_no_rank), style = MaterialTheme.typography.bodySmall)
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
    Column(
        Modifier.fillMaxSize().background(MaterialTheme.colorScheme.surface.copy(alpha = .97f))
            .verticalScroll(rememberScrollState()).padding(20.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp, Alignment.CenterVertically),
    ) {
        Text(stringResource(when (phase) {
            GamePhase.READY -> R.string.games_how_to
            GamePhase.PAUSED -> R.string.games_paused
            else -> R.string.games_finish
        }), style = MaterialTheme.typography.headlineSmall, modifier = Modifier.semantics { heading() })
        when (phase) {
            GamePhase.READY -> {
                Text(stringResource(if (session.gameId == GameId.FLIGHT) R.string.games_flight_tutorial else R.string.games_run_tutorial))
                Text(stringResource(R.string.games_limit_notice), style = MaterialTheme.typography.bodySmall)
                Button(onStart, Modifier.fillMaxWidth().testTag("games_start")) { Text(stringResource(R.string.games_start)) }
            }
            GamePhase.PAUSED -> {
                Text(stringResource(R.string.games_pause_notice))
                Button(onResume, Modifier.fillMaxWidth().testTag("games_resume")) { Text(stringResource(R.string.games_continue)) }
                OutlinedButton(onRestart, Modifier.fillMaxWidth()) { Text(stringResource(R.string.games_restart)) }
            }
            GamePhase.FINISHED -> {
                Text(stringResource(if (session.simulation.end == RunEnd.TIME_LIMIT) R.string.games_time_limit else R.string.games_collision))
                Text(stringResource(R.string.games_score, session.simulation.score))
                Text(stringResource(R.string.games_local_best, best))
                when (saveState) {
                    SaveState.SAVING -> Text(stringResource(R.string.games_saving))
                    SaveState.FAILED -> {
                        Text(stringResource(R.string.games_save_failed))
                        OutlinedButton(onRetrySave, Modifier.fillMaxWidth()) { Text(stringResource(R.string.games_retry_save)) }
                    }
                    else -> Unit
                }
                Button(onRestart, Modifier.fillMaxWidth(), enabled = saveState != SaveState.SAVING) { Text(stringResource(R.string.games_restart)) }
            }
            else -> Unit
        }
        TextButton(onSelect, Modifier.fillMaxWidth()) { Text(stringResource(R.string.games_select_other)) }
    }
}

@Composable
private fun GameControls(game: GameId, phase: GamePhase, onAction: (GameAction) -> Unit, onPause: () -> Unit) {
    val running = phase == GamePhase.RUNNING
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        if (game == GameId.FLIGHT) {
            Button({ onAction(GameAction.IMPULSE) }, Modifier.fillMaxWidth().heightIn(min = 48.dp), enabled = running) {
                Text(stringResource(R.string.games_fly))
            }
        } else {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Button({ onAction(GameAction.LEFT) }, Modifier.weight(1f).heightIn(min = 48.dp), enabled = running) { Text(stringResource(R.string.games_left)) }
                Button({ onAction(GameAction.RIGHT) }, Modifier.weight(1f).heightIn(min = 48.dp), enabled = running) { Text(stringResource(R.string.games_right)) }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Button({ onAction(GameAction.JUMP) }, Modifier.weight(1f).heightIn(min = 48.dp), enabled = running) { Text(stringResource(R.string.games_jump)) }
                Button({ onAction(GameAction.SLIDE) }, Modifier.weight(1f).heightIn(min = 48.dp), enabled = running) { Text(stringResource(R.string.games_slide)) }
            }
        }
        OutlinedButton(onPause, Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("games_pause"), enabled = running) {
            Text(stringResource(R.string.games_pause))
        }
    }
}

@Composable
private fun gameTitle(game: GameId): String = stringResource(if (game == GameId.FLIGHT) R.string.games_flight_title else R.string.games_run_title)
