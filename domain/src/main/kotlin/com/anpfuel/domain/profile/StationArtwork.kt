package com.anpfuel.domain.profile

/** Local artwork identity, independent of ownership, brand or partnership. */
enum class StationArtworkAsset { ABASTEVO_ICON, ABASTEVO_BANNER }

data class StationArtwork(
    val icon: StationArtworkAsset = StationArtworkAsset.ABASTEVO_ICON,
    val banner: StationArtworkAsset = StationArtworkAsset.ABASTEVO_BANNER,
) {
    companion object { val BETA_DEFAULT = StationArtwork() }
}
