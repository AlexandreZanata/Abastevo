package com.anpfuel.app.games

import com.anpfuel.domain.games.GameAction
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class GameplayInputTest {
    @Test fun shortTouchesAndCancelledMotionDoNothing() {
        assertEquals(GameAction.NONE, GameplayInput.swipe(12f, 23f, 24f))
        assertEquals(GameAction.NONE, GameplayInput.swipe(0f, 0f, 24f))
    }
    @Test fun horizontalThresholdChangesOnlyOneLane() {
        assertEquals(GameAction.LEFT, GameplayInput.swipe(-24f, 0f, 24f))
        assertEquals(GameAction.RIGHT, GameplayInput.swipe(24f, 24f, 24f))
    }
    @Test fun dominantVerticalSwipeJumpsOrSlides() {
        assertEquals(GameAction.JUMP, GameplayInput.swipe(50f, -100f, 24f))
        assertEquals(GameAction.SLIDE, GameplayInput.swipe(-50f, 100f, 24f))
    }
    @Test fun invalidMotionNeverCreatesAction() {
        assertEquals(GameAction.NONE, GameplayInput.swipe(Float.NaN, 200f, 24f))
        assertEquals(GameAction.NONE, GameplayInput.swipe(200f, Float.POSITIVE_INFINITY, 24f))
        assertEquals(GameAction.NONE, GameplayInput.swipe(200f, 0f, 0f))
    }
}
