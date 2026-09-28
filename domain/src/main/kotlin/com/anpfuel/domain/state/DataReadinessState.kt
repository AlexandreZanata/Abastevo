package com.anpfuel.domain.state

enum class DataReadinessState {
    EMPTY,
    SYNCING,
    PARTIAL,
    READY,
    STALE,
    ERROR,
}
