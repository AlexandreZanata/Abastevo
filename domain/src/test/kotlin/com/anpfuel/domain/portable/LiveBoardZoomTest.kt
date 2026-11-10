package com.anpfuel.domain.portable

import com.anpfuel.domain.portable.LiveBoardZoom.Board
import com.anpfuel.domain.portable.LiveBoardZoom.State
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

/** Synthetic board rectangles only; no preview frame or photo-derived data. */
class LiveBoardZoomTest {
    private val centred = Board(0.4, 0.45, 0.6, 0.55)

    /** Feeds the same reading [times] times, as consecutive preview analyses do. */
    private fun run(start: State, board: Board?, times: Int, min: Double = 1.0, max: Double = 8.0): State =
        (1..times).fold(start) { state, _ -> LiveBoardZoom.next(state, board, min, max) }

    @Test fun `a centred board is zoomed until it fills most of the frame`() {
        val state = run(State(1.0), centred, 2)
        assertEquals(4.0, state.ratio, 1e-9)
    }

    @Test fun `a single sighting does not move the zoom`() {
        assertEquals(1.0, LiveBoardZoom.next(State(1.0), centred, 1.0, 8.0).ratio)
    }

    @Test fun `the zoom stays within the device range and the product ceiling`() {
        assertEquals(2.5, run(State(1.0), centred, 2, max = 2.5).ratio, 1e-9)
        val tiny = Board(0.48, 0.49, 0.52, 0.51)
        assertEquals(LiveBoardZoom.MAX_RATIO, run(State(1.0), tiny, 2, max = 10.0).ratio, 1e-9)
    }

    @Test fun `an oversized board zooms out but never below the wide ratio`() {
        val full = Board(0.0, 0.1, 1.0, 0.9)
        assertEquals(2.4, run(State(3.0), full, 2).ratio, 1e-9)
        assertEquals(1.0, run(State(1.0), full, 2, min = 0.6).ratio, 1e-9)
        assertEquals(1.2, run(State(1.2), full, 2, min = 1.2).ratio, 1e-9)
    }

    @Test fun `an off-centre board is only zoomed while it stays inside the frame`() {
        val nearEdge = Board(0.05, 0.45, 0.25, 0.55)
        assertEquals(1.0, run(State(1.0), nearEdge, 2).ratio, 1e-9)
        val offCentre = Board(0.3, 0.45, 0.45, 0.55)
        assertEquals(LiveBoardZoom.EDGE_LIMIT / 0.4, run(State(1.0), offCentre, 2).ratio, 1e-9)
    }

    @Test fun `small differences keep the zoom still`() {
        val almost = Board(0.15, 0.45, 0.85, 0.55)
        assertEquals(2.0, run(State(2.0), almost, 5).ratio, 1e-9)
        val smaller = Board(0.2, 0.45, 0.8, 0.55)
        assertEquals(2.0 * 0.8 / 0.6, run(State(2.0), smaller, 2).ratio, 1e-9)
    }

    @Test fun `a board near a bound still reaches the bound and then holds`() {
        val small = Board(0.42, 0.45, 0.58, 0.55)
        val atCeiling = run(State(4.6), small, 2, max = 5.0)
        assertEquals(5.0, atCeiling.ratio, 1e-9)
        assertEquals(5.0, run(atCeiling, Board(0.415, 0.45, 0.585, 0.55), 4, max = 5.0).ratio, 1e-9)
    }

    @Test fun `a lost board holds the zoom for a while then returns to wide`() {
        val zoomed = run(State(1.0), centred, 2)
        val held = run(zoomed, null, LiveBoardZoom.RELEASE_ANALYSES - 1)
        assertEquals(4.0, held.ratio, 1e-9)
        assertEquals(4.0, run(LiveBoardZoom.next(held, centred, 1.0, 8.0), null, LiveBoardZoom.RELEASE_ANALYSES - 1).ratio, 1e-9)
        assertEquals(1.0, LiveBoardZoom.next(held, null, 1.0, 8.0).ratio, 1e-9)
        assertEquals(1.3, run(State(4.0), null, LiveBoardZoom.RELEASE_ANALYSES, min = 1.3).ratio, 1e-9)
    }

    @Test fun `a board outside the frame fractions is rejected`() {
        assertThrows(IllegalArgumentException::class.java) { Board(0.6, 0.1, 0.4, 0.5) }
        assertThrows(IllegalArgumentException::class.java) { Board(-0.1, 0.1, 0.4, 1.2) }
    }
}
