package com.anpfuel.app.games

import android.graphics.Canvas
import android.graphics.Color
import android.graphics.LinearGradient
import android.graphics.Paint
import android.graphics.Path
import android.graphics.Shader
import android.graphics.Typeface
import com.anpfuel.domain.games.ArcadeRules
import com.anpfuel.domain.games.ArcadeSimulation
import com.anpfuel.domain.games.FuelFlight
import com.anpfuel.domain.games.FuelRun
import com.anpfuel.domain.games.GameId
import com.anpfuel.domain.games.RunEntityKind

/** Original vector scenery shared by live play and static previews. No bitmap/asset download. */
internal class ArcadeScene {
    private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
    private val path = Path()
    private val sky = LinearGradient(0f, 0f, 0f, 1600f, intArrayOf(NAVY, OCEAN, MIST), floatArrayOf(0f, .48f, 1f), Shader.TileMode.CLAMP)
    private val dusk = LinearGradient(0f, 0f, 0f, 1600f, intArrayOf(NAVY, INDIGO, OCEAN), floatArrayOf(0f, .42f, 1f), Shader.TileMode.CLAMP)
    private val asphalt = LinearGradient(0f, 340f, 0f, 1600f, ROAD, NAVY, Shader.TileMode.CLAMP)
    private val bold = Typeface.create("sans-serif-medium", Typeface.NORMAL)
    private var previousScore = 0
    private var pickupTick = -100

    fun draw(canvas: Canvas, game: ArcadeSimulation, alpha: Float, reducedMotion: Boolean, actionAge: Int) {
        if (game.score > previousScore) pickupTick = game.tick
        previousScore = game.score
        when (game) {
            is FuelFlight -> flight(canvas, game, alpha, reducedMotion, actionAge)
            is FuelRun -> runner(canvas, game, alpha, reducedMotion, actionAge)
        }
        if (!reducedMotion && game.tick - pickupTick in 0..16) {
            paint.color = Color.argb((16 - (game.tick - pickupTick)) * 6, 255, 215, 115)
            canvas.drawRoundRect(24f, 24f, 976f, 152f, 32f, 32f, paint)
        }
    }

    fun hud(canvas: Canvas, text: String, tick: Int) {
        box(canvas, 28f, 28f, 972f, 144f, 28f, PANEL)
        box(canvas, 48f, 52f, 56f, 116f, 4f, CYAN)
        paint.color = WHITE; paint.typeface = bold; paint.textAlign = Paint.Align.LEFT
        // Fit localized labels without clipping, independent of the simulation.
        paint.textSize = 42f
        val width = paint.measureText(text)
        if (width > 830f) paint.textSize *= 830f / width
        canvas.drawText(text, 80f, 99f, paint)
        box(canvas, 60f, 124f, 940f, 129f, 2f, ROAD)
        box(canvas, 60f, 124f, 60f + 880f * tick / ArcadeRules.MAX_TICKS, 129f, 2f, GOLD)
    }

    /** Static, landscape composition; intentionally no frame callbacks or gameplay session. */
    fun preview(canvas: Canvas, game: GameId) {
        background(canvas, game == GameId.FLIGHT, 0f)
        if (game == GameId.FLIGHT) {
            pump(canvas, 690f, -250f, 160f)
            pump(canvas, 690f, 390f, 900f)
            collectible(canvas, 755f, 275f, 30f)
            trail(canvas, 295f, 270f, 50f)
            mascot(canvas, 330f, 270f, 59f)
            cloud(canvas, 90f, 120f, 1.2f)
        } else {
            road(canvas, 170f, 600f, 0f)
            station(canvas, 150f, 240f, .7f)
            obstacle(canvas, RunEntityKind.CONE, 730f, 415f, 46f)
            collectible(canvas, 505f, 260f, 22f)
            shadow(canvas, 400f, 485f, 65f, 16f)
            mascot(canvas, 400f, 414f, 53f)
        }
    }

    private fun background(canvas: Canvas, flight: Boolean, drift: Float) {
        paint.shader = if (flight) sky else dusk
        canvas.drawRect(0f, 0f, 1000f, 1600f, paint); paint.shader = null
        paint.color = HALO; canvas.drawCircle(805f, 235f, 115f, paint)
        paint.color = GOLD; canvas.drawCircle(805f, 235f, 72f, paint)
        paint.color = STAR
        repeat(11) { i -> canvas.drawCircle(55f + (i * 137 % 910), 175f + (i * 89 % 260), if (i % 3 == 0) 3f else 2f, paint) }
        cloud(canvas, 110f - drift * .3f, 400f, 1.4f)
        cloud(canvas, 590f - drift * .2f, 545f, 1f)
    }

    private fun cloud(canvas: Canvas, x: Float, y: Float, scale: Float) {
        paint.color = CLOUD
        canvas.drawOval(x - 90 * scale, y - 15 * scale, x + 160 * scale, y + 25 * scale, paint)
        canvas.drawOval(x - 25 * scale, y - 48 * scale, x + 85 * scale, y + 20 * scale, paint)
    }

    private fun flight(canvas: Canvas, game: FuelFlight, alpha: Float, reduced: Boolean, actionAge: Int) {
        val drift = if (reduced) 0f else game.tick % 900 * .25f
        background(canvas, true, drift)
        paint.color = HILL_FAR
        path.reset(); path.moveTo(0f, 1250f)
        path.cubicTo(250f, 900f, 400f, 1400f, 700f, 1030f)
        path.cubicTo(900f, 850f, 1100f, 1200f, 1100f, 1280f)
        path.lineTo(1100f, 1500f); path.lineTo(0f, 1500f); path.close(); canvas.drawPath(path, paint)
        paint.color = HILL_NEAR
        path.reset(); path.moveTo(0f, 1390f); path.cubicTo(320f, 1130f, 580f, 1510f, 1000f, 1270f)
        path.lineTo(1000f, 1500f); path.lineTo(0f, 1500f); path.close(); canvas.drawPath(path, paint)
        for (gate in game.gates) {
            val x = gate.x + 6 * (1 - alpha)
            pump(canvas, x, 0f, gate.center - 240f)
            pump(canvas, x, gate.center + 240f, 1500f)
            if (!gate.collected) collectible(canvas, x + 65, gate.center.toFloat(), 28f)
        }
        box(canvas, 0f, 1500f, 1000f, 1600f, 0f, NAVY)
        box(canvas, 0f, 1500f, 1000f, 1512f, 0f, CYAN)
        repeat(10) { i -> box(canvas, i * 120f - drift % 120, 1544f, i * 120f + 65 - drift % 120, 1552f, 3f, GOLD) }
        val y = game.y - game.velocity * (1 - alpha)
        if (!reduced) trail(canvas, 188f, y, 34f)
        feedback(canvas, 220f, y, actionAge, reduced)
        mascot(canvas, 220f, y, 42f)
    }

    private fun pump(canvas: Canvas, x: Float, top: Float, bottom: Float) {
        box(canvas, x, top, x + 130, bottom, 12f, NAVY)
        box(canvas, x + 8, top + 8, x + 110, bottom - 8, 8f, PUMP)
        box(canvas, x + 14, top + 10, x + 24, bottom - 10, 4f, CYAN)
        val panel = if (top <= 0f) bottom - 134 else top + 36
        box(canvas, x + 34, panel, x + 96, panel + 66, 9f, PANEL)
        repeat(3) { i -> box(canvas, x + 43, panel + 14 + i * 15, x + 83, panel + 19 + i * 15, 2f, MINT) }
        val lip = if (top <= 0f) bottom - 20 else top
        box(canvas, x, lip, x + 130, lip + 20, 4f, GOLD)
        paint.color = NAVY
        repeat(4) { i -> canvas.drawRect(x + 10 + i * 32, lip + 4, x + 22 + i * 32, lip + 16, paint) }
        paint.color = WHITE
        canvas.drawCircle(x + 44, panel + 90, 6f, paint)
        canvas.drawCircle(x + 84, panel + 90, 6f, paint)
    }

    private fun runner(canvas: Canvas, game: FuelRun, alpha: Float, reduced: Boolean, actionAge: Int) {
        background(canvas, false, 0f)
        val drift = if (reduced) 0f else game.tick % 75 / 75f
        box(canvas, 0f, 480f, 1000f, 1600f, 0f, HILL_NEAR)
        station(canvas, 85f, 570f, .7f)
        station(canvas, 880f, 510f, .45f)
        road(canvas, 340f, 1520f, drift)
        for (i in game.entities.indices.reversed()) {
            val entity = game.entities[i]
            val depth = entity.depth + game.speed * (1 - alpha)
            val p = (1f - depth / 1800f).coerceIn(0f, 1f)
            val x = 500f + entity.lane * (100 + 220 * p)
            val y = 340f + 1040 * p
            val size = 20f + 84 * p
            if (entity.kind == RunEntityKind.FUEL) {
                if (!entity.crossed) collectible(canvas, x, y, size * .55f)
            } else obstacle(canvas, entity.kind, x, y, size)
        }
        val x = 500f + game.lane * 300f
        val jump = if (game.jumpTicks > 0) 180f * (1 - (game.jumpTicks / 24f - 1) * (game.jumpTicks / 24f - 1)) else 0f
        val y = 1380f - jump
        shadow(canvas, x, 1442f, 80f - jump * .15f, 18f)
        feedback(canvas, x, y - 28, actionAge, reduced)
        if (game.slideTicks > 0) {
            box(canvas, x - 68, y - 30, x + 68, y + 30, 30f, CYAN)
            box(canvas, x - 35, y - 14, x + 52, y + 5, 9f, NAVY)
            box(canvas, x - 70, y + 36, x + 70, y + 44, 4f, GOLD)
        } else {
            val stride = if (reduced || game.jumpTicks > 0) 0f else if (game.tick % 20 < 10) 12f else -12f
            box(canvas, x - 35, y + 16, x - 10, y + 64 + stride, 10f, WHITE)
            box(canvas, x + 10, y + 16, x + 35, y + 64 - stride, 10f, WHITE)
            box(canvas, x - 45, y + 53 + stride, x - 8, y + 67 + stride, 6f, GOLD)
            box(canvas, x + 8, y + 53 - stride, x + 45, y + 67 - stride, 6f, GOLD)
            mascot(canvas, x, y - 28, 56f)
        }
    }

    private fun road(canvas: Canvas, horizon: Float, bottom: Float, drift: Float) {
        path.reset(); path.moveTo(350f, horizon); path.lineTo(650f, horizon)
        path.lineTo(980f, bottom); path.lineTo(20f, bottom); path.close()
        paint.shader = asphalt; canvas.drawPath(path, paint); paint.shader = null
        paint.color = CYAN; paint.strokeWidth = 8f
        canvas.drawLine(350f, horizon, 20f, bottom, paint); canvas.drawLine(650f, horizon, 980f, bottom, paint)
        repeat(9) { i ->
            val p = (i + drift) / 9f
            val q = (p + .045f).coerceAtMost(1f)
            paint.color = STRIPE; paint.strokeWidth = 2 + 9 * p
            canvas.drawLine(450f - 110 * p, horizon + (bottom - horizon) * p, 450f - 110 * q, horizon + (bottom - horizon) * q, paint)
            canvas.drawLine(550f + 110 * p, horizon + (bottom - horizon) * p, 550f + 110 * q, horizon + (bottom - horizon) * q, paint)
            // Roadside markers reinforce speed/depth without obscuring obstacles.
            val y = horizon + (bottom - horizon) * p
            box(canvas, 340f - 330 * p, y - 30 * p, 344f - 330 * p, y + 12 * p, 2f, GOLD)
            box(canvas, 656f + 330 * p, y - 30 * p, 660f + 330 * p, y + 12 * p, 2f, GOLD)
        }
    }

    private fun station(canvas: Canvas, x: Float, y: Float, scale: Float) {
        box(canvas, x - 100 * scale, y, x + 100 * scale, y + 95 * scale, 8f, NAVY)
        box(canvas, x - 110 * scale, y - 12 * scale, x + 110 * scale, y + 6 * scale, 6f, MINT)
        box(canvas, x - 78 * scale, y + 22 * scale, x + 78 * scale, y + 46 * scale, 4f, OCEAN)
        box(canvas, x - 10 * scale, y + 54 * scale, x + 38 * scale, y + 95 * scale, 4f, PUMP)
    }

    private fun obstacle(canvas: Canvas, kind: RunEntityKind, x: Float, y: Float, size: Float) {
        shadow(canvas, x, y + size, size, size * .2f)
        when (kind) {
            RunEntityKind.CONE -> {
                box(canvas, x - size, y + size * .72f, x + size, y + size, 6f, NAVY)
                path.reset(); path.moveTo(x, y - size); path.lineTo(x + size * .8f, y + size * .8f)
                path.lineTo(x - size * .8f, y + size * .8f); path.close()
                paint.color = ORANGE; canvas.drawPath(path, paint)
                paint.color = WHITE
                canvas.drawRect(x - size * .35f, y - size * .05f, x + size * .35f, y + size * .22f, paint)
            }
            RunEntityKind.BEAM -> {
                box(canvas, x - size, y - size, x - size * .8f, y + size, 4f, WHITE)
                box(canvas, x + size * .8f, y - size, x + size, y + size, 4f, WHITE)
                box(canvas, x - size, y - size * 1.5f, x + size, y - size * .75f, 6f, GOLD)
                paint.color = NAVY
                repeat(4) { i -> canvas.drawRect(x - size * .85f + i * size * .48f, y - size * 1.4f, x - size * .65f + i * size * .48f, y - size * .85f, paint) }
            }
            RunEntityKind.TRUCK -> {
                box(canvas, x - size, y - size * 1.7f, x + size, y + size, 12f, INDIGO)
                box(canvas, x - size * .82f, y - size * 1.5f, x + size * .82f, y - size * .4f, 8f, MINT)
                box(canvas, x - size * .62f, y - size * 1.35f, x + size * .62f, y - size * .62f, 5f, PANEL)
                box(canvas, x - size * .7f, y + size * .4f, x + size * .7f, y + size * .58f, 3f, WHITE)
                box(canvas, x - size * .85f, y + size * .72f, x - size * .5f, y + size * .86f, 3f, GOLD)
                box(canvas, x + size * .5f, y + size * .72f, x + size * .85f, y + size * .86f, 3f, GOLD)
            }
            RunEntityKind.FUEL -> Unit
        }
    }

    private fun collectible(canvas: Canvas, x: Float, y: Float, radius: Float) {
        paint.color = HALO; canvas.drawCircle(x, y, radius * 1.6f, paint)
        drop(canvas, x, y, radius, GOLD)
        paint.color = WHITE; paint.strokeWidth = 3f
        canvas.drawLine(x - radius * .22f, y + radius * .2f, x + radius * .22f, y + radius * .2f, paint)
        canvas.drawLine(x, y, x, y + radius * .4f, paint)
    }

    private fun mascot(canvas: Canvas, x: Float, y: Float, radius: Float) {
        drop(canvas, x, y, radius, CYAN)
        box(canvas, x - radius * .64f, y - radius * .15f, x + radius * .64f, y + radius * .35f, radius * .22f, NAVY)
        paint.color = WHITE
        canvas.drawCircle(x - radius * .25f, y + radius * .09f, radius * .08f, paint)
        canvas.drawCircle(x + radius * .25f, y + radius * .09f, radius * .08f, paint)
        box(canvas, x - radius * .19f, y + radius * .56f, x + radius * .19f, y + radius * .63f, 3f, NAVY)
    }

    private fun drop(canvas: Canvas, x: Float, y: Float, radius: Float, color: Int) {
        path.reset(); path.moveTo(x, y - radius)
        path.cubicTo(x + radius * 1.6f, y + radius * .35f, x + radius * .65f, y + radius, x, y + radius)
        path.cubicTo(x - radius * .65f, y + radius, x - radius * 1.6f, y + radius * .35f, x, y - radius)
        paint.color = NAVY; paint.style = Paint.Style.STROKE; paint.strokeWidth = 5f; canvas.drawPath(path, paint)
        paint.style = Paint.Style.FILL; paint.color = color; canvas.drawPath(path, paint)
        paint.color = WHITE; canvas.drawOval(x - radius * .35f, y - radius * .35f, x - radius * .12f, y - radius * .02f, paint)
    }

    private fun trail(canvas: Canvas, x: Float, y: Float, radius: Float) {
        repeat(4) { i ->
            paint.color = Color.argb(75 - i * 16, 83, 223, 227)
            canvas.drawCircle(x - i * 34, y + i * 4, radius * (1 - i * .17f), paint)
        }
    }

    private fun feedback(canvas: Canvas, x: Float, y: Float, age: Int, reduced: Boolean) {
        if (reduced || age !in 0..10) return
        paint.color = Color.argb(150 - age * 13, 255, 255, 255)
        paint.style = Paint.Style.STROKE; paint.strokeWidth = 4f
        canvas.drawCircle(x, y, 65f + age * 5f, paint); paint.style = Paint.Style.FILL
    }

    private fun shadow(canvas: Canvas, x: Float, y: Float, width: Float, height: Float) {
        paint.color = SHADOW; canvas.drawOval(x - width, y - height, x + width, y + height, paint)
    }

    private fun box(canvas: Canvas, left: Float, top: Float, right: Float, bottom: Float, radius: Float, color: Int) {
        paint.color = color; canvas.drawRoundRect(left, top, right, bottom, radius, radius, paint)
    }

    companion object {
        val NAVY = Color.rgb(11, 24, 48)
        private val PANEL = Color.rgb(17, 35, 61)
        private val OCEAN = Color.rgb(33, 95, 115)
        private val MIST = Color.rgb(192, 230, 214)
        private val INDIGO = Color.rgb(58, 64, 111)
        private val ROAD = Color.rgb(55, 78, 103)
        private val PUMP = Color.rgb(40, 124, 144)
        private val HILL_FAR = Color.rgb(53, 120, 137)
        private val HILL_NEAR = Color.rgb(27, 83, 92)
        private val CYAN = Color.rgb(83, 223, 227)
        private val MINT = Color.rgb(163, 237, 206)
        private val GOLD = Color.rgb(255, 215, 115)
        private val ORANGE = Color.rgb(250, 137, 83)
        private val WHITE = Color.rgb(245, 250, 255)
        private val CLOUD = Color.argb(25, 245, 250, 255)
        private val STAR = Color.argb(130, 245, 250, 255)
        private val HALO = Color.argb(30, 255, 215, 115)
        private val STRIPE = Color.argb(180, 163, 237, 206)
        private val SHADOW = Color.argb(70, 0, 0, 0)
    }
}
