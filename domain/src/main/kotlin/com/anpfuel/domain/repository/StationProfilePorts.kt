package com.anpfuel.domain.repository

import com.anpfuel.domain.profile.StationProfile

/**
 * P32-T01 — Anonymous station-profile read port.
 *
 * Server remains authoritative for identity and grants; the gateway
 * returns the public projection only (see `StationProfile`). Null means
 * unknown/unclaimed — never a synthetic profile.
 */
interface StationProfileGateway {
    suspend fun getProfile(stationId: String): StationProfile?
}
