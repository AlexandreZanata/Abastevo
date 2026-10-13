package com.anpfuel.application.port

/** Resolves the selected city through the existing IBGE catalog; null means unavailable. */
fun interface StationDirectoryScopeProvider {
    suspend fun municipalityCode(): String?
}
