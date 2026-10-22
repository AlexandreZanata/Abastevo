package com.anpfuel.domain.valueobject

/**
 * Energy propulsion variant (EE-19, mirrors backend EE-10 plus MHEV).
 * Absent/legacy means ICE. MHEV behaves like HEV here (tank without
 * external charge); only BEV and PHEV carry batteries and connectors.
 */
enum class Propulsion {
    ICE,
    BEV,
    PHEV,
    HEV,
    MHEV,
}
