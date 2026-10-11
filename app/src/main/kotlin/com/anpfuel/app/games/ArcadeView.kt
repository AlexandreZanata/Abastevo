package com.anpfuel.app.games

import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.Path
import android.media.AudioManager
import android.media.ToneGenerator
import android.view.Choreographer
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.View
import com.anpfuel.app.R
import com.anpfuel.domain.games.ArcadeSession
import com.anpfuel.domain.games.FuelFlight
import com.anpfuel.domain.games.FuelRun
import com.anpfuel.domain.games.GameAction
import com.anpfuel.domain.games.GameId
import com.anpfuel.domain.games.GamePhase
import com.anpfuel.domain.games.RunEntityKind
import kotlin.math.min

/** Display-synchronized, bounded native rendering; no Compose work per simulation tick. */
class ArcadeView(context: Context, val session: ArcadeSession) : View(context), Choreographer.FrameCallback {
    var onPhaseChanged: (GamePhase) -> Unit = {}
    var reducedMotion = false
    var lowPower = false
    var muted = true
        set(value) {
            field = value
            if (value) releaseTone()
        }
    private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
    private val path = Path()
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
    fun restart() { session.restart(); changed(); schedule(); invalidate() }
    fun action(action: GameAction) { session.enqueue(action) }

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
        canvas.drawColor(NAVY)
        val scale = min(width / 1000f, height / 1600f)
        canvas.save()
        canvas.translate((width - 1000 * scale) / 2, (height - 1600 * scale) / 2)
        canvas.scale(scale, scale)
        canvas.clipRect(0f, 0f, 1000f, 1600f)
        val alpha = if (session.phase == GamePhase.RUNNING) session.interpolation else 1f
        when (val game = session.simulation) {
            is FuelFlight -> flight(canvas, game, alpha)
            is FuelRun -> runner(canvas, game, alpha)
        }
        val score = session.simulation.score
        val seconds = session.simulation.tick / 60
        if (cachedScore != score || cachedSeconds != seconds) {
            cachedScore = score; cachedSeconds = seconds
            hud = context.getString(R.string.games_hud, score, seconds)
        }
        paint.color = NAVY; canvas.drawRoundRect(30f, 24f, 970f, 134f, 24f, 24f, paint)
        paint.color = Color.WHITE; paint.textSize = 45f; paint.textAlign = Paint.Align.LEFT
        canvas.drawText(hud, 60f, 96f, paint)
        canvas.restore()
    }

    private fun flight(canvas: Canvas, game: FuelFlight, alpha: Float) {
        paint.color = SKY; canvas.drawRect(0f, 0f, 1000f, 1600f, paint)
        paint.color = CLOUD
        val drift = if (reducedMotion) 0f else game.tick % 1300 * 0.6f
        repeat(4) { i ->
            val x = (i * 350f + 1300f - drift) % 1300f - 150f
            canvas.drawOval(x, 200f + i * 240f, x + 220f, 270f + i * 240f, paint)
        }
        for (gate in game.gates) {
            val x = gate.x + 6 * (1 - alpha)
            pump(canvas, x, 0f, gate.center - 240f)
            pump(canvas, x, gate.center + 240f, 1500f)
            if (!gate.collected) drop(canvas, x + 65, gate.center.toFloat(), 28f, GOLD)
        }
        paint.color = GREEN; canvas.drawRect(0f, 1500f, 1000f, 1525f, paint)
        paint.color = ROAD; canvas.drawRect(0f, 1525f, 1000f, 1600f, paint)
        drop(canvas, 220f, game.y - game.velocity * (1 - alpha), 42f, BLUE)
    }

    private fun pump(canvas: Canvas, x: Float, top: Float, bottom: Float) {
        paint.color = NAVY; canvas.drawRoundRect(x, top, x + 130, bottom, 12f, 12f, paint)
        paint.color = Color.WHITE; canvas.drawRect(x + 12, top + 12, x + 118, bottom - 12, paint)
        paint.color = BLUE
        if (bottom - top > 140) {
            val panelY = if (top == 0f) bottom - 112 else top + 30
            canvas.drawRoundRect(x + 26, panelY, x + 104, panelY + 68, 10f, 10f, paint)
        }
    }

    private fun runner(canvas: Canvas, game: FuelRun, alpha: Float) {
        paint.color = SKY; canvas.drawRect(0f, 0f, 1000f, 1600f, paint)
        paint.color = GREEN; canvas.drawRect(0f, 340f, 1000f, 1600f, paint)
        path.reset(); path.moveTo(350f, 340f); path.lineTo(650f, 340f)
        path.lineTo(980f, 1520f); path.lineTo(20f, 1520f); path.close()
        paint.color = ROAD; canvas.drawPath(path, paint)
        paint.color = Color.WHITE; paint.strokeWidth = 6f
        canvas.drawLine(450f, 340f, 340f, 1520f, paint)
        canvas.drawLine(550f, 340f, 660f, 1520f, paint)
        val drift = if (reducedMotion) 0f else (game.tick % 60) / 60f
        paint.color = GOLD
        repeat(7) { i ->
            val p = ((i + drift) / 7f)
            canvas.drawRect(494f, 340f + p * 1180, 506f, 355f + p * 1180, paint)
        }
        // Farthest first; entities remain bounded and naturally depth ordered.
        for (index in game.entities.indices.reversed()) {
            val entity = game.entities[index]
            val depth = entity.depth + game.speed * (1 - alpha)
            val p = (1f - depth / 1800f).coerceIn(0f, 1f)
            val x = 500f + entity.lane * (100 + 220 * p)
            val y = 340f + 1040 * p
            val size = 20f + 84 * p
            when (entity.kind) {
                RunEntityKind.FUEL -> if (!entity.crossed) drop(canvas, x, y, size * 0.55f, GOLD)
                RunEntityKind.CONE -> {
                    path.reset(); path.moveTo(x, y - size); path.lineTo(x + size, y + size)
                    path.lineTo(x - size, y + size); path.close()
                    paint.color = ORANGE; canvas.drawPath(path, paint)
                    paint.color = Color.WHITE; canvas.drawRect(x - size * .5f, y, x + size * .5f, y + size * .2f, paint)
                }
                RunEntityKind.BEAM -> {
                    paint.color = GOLD
                    canvas.drawRect(x - size, y - size * 1.5f, x + size, y - size * .75f, paint)
                    canvas.drawRect(x - size, y - size, x - size * .8f, y + size, paint)
                    canvas.drawRect(x + size * .8f, y - size, x + size, y + size, paint)
                }
                RunEntityKind.TRUCK -> {
                    paint.color = PURPLE; canvas.drawRoundRect(x - size, y - size * 1.7f, x + size, y + size, 10f, 10f, paint)
                    paint.color = SKY; canvas.drawRect(x - size * .7f, y - size * 1.3f, x + size * .7f, y - size * .4f, paint)
                    paint.color = NAVY; canvas.drawCircle(x - size * .7f, y + size, size * .25f, paint)
                    canvas.drawCircle(x + size * .7f, y + size, size * .25f, paint)
                }
            }
        }
        val x = 500f + game.lane * 300f
        val jump = if (game.jumpTicks > 0) 180f * (1 - (game.jumpTicks / 24f - 1) * (game.jumpTicks / 24f - 1)) else 0f
        val y = 1380f - jump
        if (game.slideTicks > 0) {
            paint.color = BLUE; canvas.drawRoundRect(x - 65, y - 28, x + 65, y + 28, 28f, 28f, paint)
        } else {
            paint.color = NAVY; canvas.drawRect(x - 38, y + 20, x - 10, y + 65, paint)
            canvas.drawRect(x + 10, y + 20, x + 38, y + 65, paint)
            drop(canvas, x, y - 28, 56f, BLUE)
        }
    }

    private fun drop(canvas: Canvas, x: Float, y: Float, radius: Float, color: Int) {
        path.reset(); path.moveTo(x, y - radius)
        path.cubicTo(x + radius * 1.7f, y + radius * .5f, x + radius * .6f, y + radius, x, y + radius)
        path.cubicTo(x - radius * .6f, y + radius, x - radius * 1.7f, y + radius * .5f, x, y - radius)
        paint.color = NAVY; paint.style = Paint.Style.STROKE; paint.strokeWidth = 5f; canvas.drawPath(path, paint)
        paint.style = Paint.Style.FILL; paint.color = color; canvas.drawPath(path, paint)
        paint.color = Color.WHITE; canvas.drawOval(x - radius * .3f, y - radius * .2f, x, y + radius * .4f, paint)
    }

    private companion object {
        val NAVY = Color.rgb(14, 29, 57)
        val SKY = Color.rgb(215, 239, 249)
        val CLOUD = Color.rgb(247, 252, 255)
        val BLUE = Color.rgb(23, 94, 222)
        val GOLD = Color.rgb(249, 197, 58)
        val GREEN = Color.rgb(100, 166, 122)
        val ROAD = Color.rgb(65, 79, 99)
        val ORANGE = Color.rgb(220, 85, 36)
        val PURPLE = Color.rgb(123, 83, 160)
    }
}
