package com.anpfuel.app.games

import android.content.Context
import android.graphics.Canvas
import android.media.AudioManager
import android.media.ToneGenerator
import android.view.Choreographer
import android.view.HapticFeedbackConstants
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.View
import com.anpfuel.app.R
import com.anpfuel.domain.games.ArcadeSession
import com.anpfuel.domain.games.GameAction
import com.anpfuel.domain.games.GameId
import com.anpfuel.domain.games.GamePhase
import kotlin.math.min

/** Display-synchronized, bounded native rendering; no Compose work per simulation tick. */
class ArcadeView(context: Context, val session: ArcadeSession) : View(context), Choreographer.FrameCallback {
    // Idle preview constructor for Android layout/accessibility tooling.
    constructor(context: Context) : this(context, ArcadeSession(GameId.FLIGHT, 1))

    var onPhaseChanged: (GamePhase) -> Unit = {}
    var reducedMotion = false
    var lowPower = false
    var muted = true
        set(value) {
            field = value
            if (value) releaseTone()
        }
    private val scene = ArcadeScene()
    private var lastActionTick = -100
    private var tone: ToneGenerator? = null
    private var scheduled = false
    private var notifiedPhase = session.phase
    private var lastDrawFrame = 0L
    private var downX = 0f
    private var downY = 0f
    private var touching = false
    private var cachedScore = -1
    private var cachedSeconds = -1
    private var hud = ""

    init {
        isFocusable = true
        isClickable = true
        contentDescription = context.getString(
            if (session.gameId == GameId.FLIGHT) R.string.games_flight_accessibility else R.string.games_run_accessibility,
        )
    }

    fun start() { session.start(); changed(); schedule() }
    fun resumeGame() { session.resume(); changed(); schedule() }
    fun restart() { lastActionTick = -100; session.restart(); changed(); schedule(); invalidate() }
    fun action(action: GameAction) {
        if (session.phase != GamePhase.RUNNING || action == GameAction.NONE) return
        session.enqueue(action)
        lastActionTick = session.simulation.tick
        // Honors the system haptic setting; no permission or vibrator service.
        performHapticFeedback(HapticFeedbackConstants.KEYBOARD_TAP)
    }

    fun pause() {
        session.pause()
        Choreographer.getInstance().removeFrameCallback(this)
        scheduled = false
        touching = false
        lastDrawFrame = 0
        releaseTone()
        changed()
        invalidate()
    }

    fun dispose() {
        onPhaseChanged = {}
        pause()
    }

    override fun onAttachedToWindow() { super.onAttachedToWindow(); schedule() }
    override fun onDetachedFromWindow() { pause(); super.onDetachedFromWindow() }
    override fun onWindowVisibilityChanged(visibility: Int) {
        super.onWindowVisibilityChanged(visibility)
        if (visibility != VISIBLE) pause()
    }

    private fun schedule() {
        if (!scheduled && isAttachedToWindow && isShown && windowVisibility == VISIBLE && session.phase == GamePhase.RUNNING) {
            scheduled = true
            Choreographer.getInstance().postFrameCallback(this)
        }
    }

    override fun doFrame(frameTimeNanos: Long) {
        scheduled = false
        if (!isAttachedToWindow || !isShown || windowVisibility != VISIBLE || session.phase != GamePhase.RUNNING) {
            pause()
            return
        }
        val oldScore = session.simulation.score
        session.frame(frameTimeNanos)
        if (session.phase == GamePhase.FINISHED) cue(ToneGenerator.TONE_PROP_NACK)
        else if (session.simulation.score > oldScore) cue(ToneGenerator.TONE_PROP_BEEP)
        changed()
        val interval = if (lowPower) 33_333_333L else 16_666_666L
        if (frameTimeNanos - lastDrawFrame >= interval || session.phase == GamePhase.FINISHED) {
            lastDrawFrame = frameTimeNanos
            invalidate()
        }
        schedule()
    }

    private fun changed() {
        if (notifiedPhase != session.phase) {
            notifiedPhase = session.phase
            onPhaseChanged(notifiedPhase)
        }
    }

    private fun cue(kind: Int) {
        if (muted) return
        // Audio output can be unavailable. Visual gameplay and scoring remain independent.
        try {
            val output = tone ?: ToneGenerator(AudioManager.STREAM_MUSIC, 25).also { tone = it }
            output.startTone(kind, 65)
        } catch (_: RuntimeException) { releaseTone() }
    }

    private fun releaseTone() { tone?.release(); tone = null }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                if (session.phase != GamePhase.RUNNING) return false
                downX = event.x; downY = event.y; touching = true
                return true
            }
            MotionEvent.ACTION_UP -> {
                if (touching) {
                    if (session.gameId == GameId.FLIGHT) performClick()
                    else action(GameplayInput.swipe(event.x - downX, event.y - downY, 24f * resources.displayMetrics.density))
                }
                touching = false
                return true
            }
            MotionEvent.ACTION_CANCEL -> { touching = false; return true }
            MotionEvent.ACTION_MOVE -> return touching
        }
        return super.onTouchEvent(event)
    }

    override fun performClick(): Boolean {
        super.performClick()
        if (session.gameId == GameId.FLIGHT) action(GameAction.IMPULSE)
        return true
    }

    override fun onKeyDown(keyCode: Int, event: KeyEvent): Boolean {
        val command = when (keyCode) {
            KeyEvent.KEYCODE_DPAD_LEFT -> GameAction.LEFT
            KeyEvent.KEYCODE_DPAD_RIGHT -> GameAction.RIGHT
            KeyEvent.KEYCODE_DPAD_DOWN -> GameAction.SLIDE
            KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_SPACE, KeyEvent.KEYCODE_ENTER ->
                if (session.gameId == GameId.FLIGHT) GameAction.IMPULSE else GameAction.JUMP
            else -> return super.onKeyDown(keyCode, event)
        }
        if (event.repeatCount == 0) action(command)
        return true
    }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        canvas.drawColor(ArcadeScene.NAVY)
        val scale = min(width / 1000f, height / 1600f)
        canvas.save()
        canvas.translate((width - 1000 * scale) / 2, (height - 1600 * scale) / 2)
        canvas.scale(scale, scale)
        canvas.clipRect(0f, 0f, 1000f, 1600f)
        val alpha = if (session.phase == GamePhase.RUNNING) session.interpolation else 1f
        scene.draw(canvas, session.simulation, alpha, reducedMotion, session.simulation.tick - lastActionTick)
        val score = session.simulation.score
        val seconds = session.simulation.tick / 60
        if (cachedScore != score || cachedSeconds != seconds) {
            cachedScore = score; cachedSeconds = seconds
            hud = context.getString(R.string.games_hud, score, seconds)
        }
        scene.hud(canvas, hud, session.simulation.tick)
        canvas.restore()
    }
}
