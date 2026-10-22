package com.anpfuel.app.navigation

import com.anpfuel.domain.valueobject.FuelProduct

object Routes {
    const val ONBOARDING = "onboarding"
    const val AUTH = "auth"
    const val ACCOUNT = "account"
    const val HOME = "home"
    const val EXPLORE = "home"
    const val COMMUNITY = "community"
    const val PROFILE = "profile"
    const val GAMES = "games"
    const val LOCATION = "location"
    const val HISTORY = "history"
    const val STATIONS = "stations"
    const val STATIONS_WITH_FUEL = "stations/{fuelProduct}"
    const val SETTINGS = "settings"
    const val HELP = "help"
    const val PRIVACY = "privacy"
    const val VEHICLES = "vehicles"
    const val VEHICLE_ENTRIES = "vehicle-entries/{vehicleId}"

    fun vehicleEntries(vehicleId: String): String {
        require(java.util.UUID.fromString(vehicleId).toString() == vehicleId)
        return "vehicle-entries/$vehicleId"
    }
    const val WEEK_PICKER = "week_picker"
    const val CAPTURE = "capture"
    const val CONTRIBUTIONS = "contributions"
    const val CAPTURE_WITH_TARGET = "capture?stationId={stationId}&fuel={fuel}"
    const val STATION_CLAIM = "station-claim/{stationId}"
    const val STATION_MANAGEMENT = "station-management/{stationId}"
    fun stationClaim(stationId: String): String = "station-claim/$stationId"
    fun stationManagement(stationId: String): String = "station-management/$stationId"

    const val STATION_PAGE = "station/{stationKey}?fuelProduct={fuelProduct}"
    fun stationPage(stationKey: String, fuel: FuelProduct): String {
        require(com.anpfuel.domain.discovery.StationPageIdentity.parse(stationKey) != null) { "invalid station identity" }
        return "station/$stationKey?fuelProduct=${fuel.name}"
    }

    const val STATION_PROFILE = "station-profile/{stationId}"

    const val STATION_QUALITY = "station-quality/{stationId}?fuelProduct={fuelProduct}"
    fun stationQuality(stationId:String,fuel:FuelProduct):String {
        require(java.util.UUID.fromString(stationId).toString()==stationId)
        return "station-quality/$stationId?fuelProduct=${fuel.name}"
    }

    const val QUALITY_REPORTS="quality-reports/{stationId}?fuelProduct={fuelProduct}&revisionId={revisionId}&sampleRef={sampleRef}"
    fun qualityReportHistory():String="quality-reports/own?fuelProduct=GASOLINE_REGULAR&revisionId=&sampleRef="
    fun qualityReports(stationId:String,fuel:FuelProduct,revisionId:String?=null,sampleRef:String?=null):String {
        require(com.anpfuel.domain.quality.qualityReportId(stationId))
        require((revisionId==null)==(sampleRef==null))
        require(revisionId==null || com.anpfuel.domain.quality.qualityReportId(revisionId))
        val sample=sampleRef?.let {java.net.URLEncoder.encode(it,"UTF-8").replace("+","%20")}.orEmpty()
        return "quality-reports/$stationId?fuelProduct=${fuel.name}&revisionId=${revisionId.orEmpty()}&sampleRef=$sample"
    }

    fun stationProfile(stationId: String): String = "station-profile/$stationId"

    const val SUGGEST = "suggest"

    const val OCCURRENCE_EXPLORER = "occurrence-explorer"
    const val OCCURRENCE_DETAIL = "occurrence-detail/{recordId}"
    fun occurrenceDetail(recordId: String): String {
        require(java.util.UUID.fromString(recordId).toString() == recordId)
        return "occurrence-detail/$recordId"
    }

    fun stations(fuelProduct: FuelProduct): String = "stations/${fuelProduct.name}"

    /** Contextual capture for one canonical station + wire fuel (P37-T01). */
    fun capture(stationId: String, fuelProductWire: String): String =
        "capture?stationId=$stationId&fuel=$fuelProductWire"
}
