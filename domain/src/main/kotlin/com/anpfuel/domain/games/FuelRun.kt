package com.anpfuel.domain.games

class FuelRun(seed: Int) : ArcadeSimulation {
    override var tick = 0
        private set
    override var score = 0
        private set
    override var end: RunEnd? = null
        private set
    var lane = 0
        private set
    var jumpTicks = 0
        private set
    var slideTicks = 0
        private set
    val speed: Int get() = (10 + tick / 900).coerceAtMost(20)
    private val random = ArcadeRandom(seed)
    private val activeEntities = ArrayList<RunEntity>(ArcadeRules.MAX_ENTITIES)
    val entities: List<RunEntity> get() = activeEntities

    override fun step(action: GameAction) {
        if (end != null) return
        tick++
        jumpTicks = (jumpTicks - 1).coerceAtLeast(0)
        slideTicks = (slideTicks - 1).coerceAtLeast(0)
        when (action) {
            GameAction.LEFT -> lane = (lane - 1).coerceAtLeast(-1)
            GameAction.RIGHT -> lane = (lane + 1).coerceAtMost(1)
            GameAction.JUMP -> if (jumpTicks == 0 && slideTicks == 0) jumpTicks = 48
            GameAction.SLIDE -> if (slideTicks == 0 && jumpTicks == 0) slideTicks = 36
            else -> Unit
        }
        if (tick >= 90 && (tick - 90) % 75 == 0 && activeEntities.size < ArcadeRules.MAX_ENTITIES) {
            activeEntities.add(RunEntity(random.next(3) - 1, RunEntityKind.entries[random.next(4)], 1800))
        }
        activeEntities.forEach { it.depth -= speed }
        if (activeEntities.any {
                !it.crossed && it.depth <= 80 && it.lane == lane && when (it.kind) {
                    RunEntityKind.FUEL -> false
                    RunEntityKind.CONE -> jumpTicks == 0
                    RunEntityKind.BEAM -> slideTicks == 0
                    RunEntityKind.TRUCK -> true
                }
            }) {
            end = RunEnd.COLLISION
            return
        }
        activeEntities.forEach {
            if (!it.crossed && it.depth <= 80) {
                if (it.lane == lane && it.kind == RunEntityKind.FUEL) score += 5
                it.crossed = true
            }
        }
        if (tick % 60 == 0) score++
        activeEntities.removeAll { it.depth <= 0 }
        if (tick == ArcadeRules.MAX_TICKS) end = RunEnd.TIME_LIMIT
    }
}

enum class RunEntityKind { FUEL, CONE, BEAM, TRUCK }

class RunEntity internal constructor(val lane: Int, val kind: RunEntityKind, depth: Int) {
    var depth = depth
        internal set
    var crossed = false
        internal set
}
