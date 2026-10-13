package com.anpfuel.app.games

import com.anpfuel.domain.games.GameAction
import kotlin.math.abs

/** One swipe, one tick action; short taps never accidentally move the runner. */
internal object GameplayInput {
    fun swipe(dx: Float, dy: Float, threshold: Float): GameAction {
        if (!dx.isFinite() || !dy.isFinite() || !threshold.isFinite() || threshold <= 0) return GameAction.NONE
        if (maxOf(abs(dx), abs(dy)) < threshold) return GameAction.NONE
        return if (abs(dx) >= abs(dy)) {
            if (dx < 0) GameAction.LEFT else GameAction.RIGHT
        } else {
            if (dy < 0) GameAction.JUMP else GameAction.SLIDE
        }
    }
}
