package com.anpfuel.domain.portable

import kotlin.math.abs
import kotlin.math.max
import kotlin.math.min

/**
 * Camera zoom while the contributor aims at the price board (B-BR-PC02 live
 * framing). It only decides how far the preview zooms; it never takes the
 * photo and never reads a price.
 */
object LiveBoardZoom {
    /** Share of the frame the board's larger side should cover. */
    const val TARGET_FILL = 0.8
    /** Farthest board edge allowed from the centre, as a share of the half frame. */
    const val EDGE_LIMIT = 0.95
    /** Same ceiling as the review screen's zoom; digital zoom beyond it only blurs the evidence. */
    const val MAX_RATIO = 5.0
    const val WIDE_RATIO = 1.0
    /** Relative change below which the zoom stays where it is. */
    const val HYSTERESIS = 0.15
    /** Consecutive analyses that must show the board before the zoom moves. */
    const val CONFIRM_ANALYSES = 2
    /** Consecutive analyses without a board before the zoom returns to wide. */
    const val RELEASE_ANALYSES = 6

    /** Board rectangle as fractions (0..1) of the analysed frame. */
    data class Board(val left: Double, val top: Double, val right: Double, val bottom: Double) {
        init {
            require(left >= 0 && top >= 0 && right <= 1 && bottom <= 1 && left < right && top < bottom) { "zoom.board" }
        }
    }

    /** Zoom ratio the camera should hold, with the counters that give it hysteresis. */
    data class State(val ratio: Double, val seen: Int = 0, val missed: Int = 0)

    /**
     * Next zoom after one analysis read at [State.ratio]. [minRatio] and [maxRatio] are the
     * device's zoom range.
     */
    fun next(state: State, board: Board?, minRatio: Double, maxRatio: Double): State {
        val wide = max(WIDE_RATIO, minRatio).coerceAtMost(maxRatio)
        val ceiling = min(maxRatio, MAX_RATIO).coerceAtLeast(wide)
        if (board == null) {
            val missed = min(state.missed + 1, RELEASE_ANALYSES)
            return State(if (missed >= RELEASE_ANALYSES) wide else state.ratio, seen = 0, missed = missed)
        }
        val seen = min(state.seen + 1, CONFIRM_ANALYSES)
        if (seen < CONFIRM_ANALYSES) return State(state.ratio, seen = seen, missed = 0)
        val fill = max(board.right - board.left, board.bottom - board.top)
        // Zoom crops around the centre: the edge farthest from it decides when the board leaves the frame.
        val extent = 2 * maxOf(abs(board.left - 0.5), abs(board.right - 0.5), abs(board.top - 0.5), abs(board.bottom - 0.5))
        val target = (state.ratio * min(TARGET_FILL / fill, EDGE_LIMIT / extent)).coerceIn(wide, ceiling)
        val atBound = target == wide || target == ceiling
        val moves = abs(target - state.ratio) / state.ratio > HYSTERESIS || (atBound && target != state.ratio)
        return State(if (moves) target else state.ratio, seen = seen, missed = 0)
    }
}
