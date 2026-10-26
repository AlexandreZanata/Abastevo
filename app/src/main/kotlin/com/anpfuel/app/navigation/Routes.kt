package com.anpfuel.app.navigation

import com.anpfuel.domain.valueobject.FuelProduct

object Routes {
    const val ONBOARDING = "onboarding"
    const val AUTH = "auth"
    const val ACCOUNT = "account"
    const val PREMIUM = "premium"
    const val PREMIUM_MANAGEMENT = "premium-management"
    const val LEDGER_SYNC = "ledger-sync"
    const val HOME = "home"
    const val EXPLORE = "home"
    const val COMMUNITY = "community"
    const val MAP = "map"
    const val MAP_CITY = "map/city"
    const val MAP_PLAN = "map/plan/{tripId}"
    fun mapPlan(tripId: String): String {
        require(java.util.UUID.fromString(tripId).toString() == tripId)
        return "map/plan/$tripId"
    }
    const val PROFILE = "profile"
    const val DRIVER_HUB = "driver-hub"
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
    const val VEHICLE_ENTRIES_ACTION = "vehicle-entries/{vehicleId}?action={action}"

    fun vehicleEntries(vehicleId: String): String {
        require(java.util.UUID.fromString(vehicleId).toString() == vehicleId)
        return "vehicle-entries/$vehicleId"
    }

    fun vehicleEntriesAction(vehicleId: String, action: String): String {
        require(java.util.UUID.fromString(vehicleId).toString() == vehicleId)
        require(action == "refuel" || action == "charge")
        return "vehicle-entries/$vehicleId?action=$action"
    }

    const val CHARGING_EXPLORE = "charging-explore/{vehicleId}"
    const val CHARGING_COMMUNITY = "charging-community/{placeId}"

    fun chargingCommunity(placeId: String): String {
        require(java.util.UUID.fromString(placeId).toString() == placeId)
        return "charging-community/$placeId"
    }

    fun chargingExplore(vehicleId: String): String {
        require(java.util.UUID.fromString(vehicleId).toString() == vehicleId)
        return "charging-explore/$vehicleId"
    }

    const val MASTER_EXPLORE = "master-explore"

    const val MONTHLY_INDICATORS = "monthly-indicators/{vehicleId}"

    fun monthlyIndicators(vehicleId: String): String {
        require(java.util.UUID.fromString(vehicleId).toString() == vehicleId)
        return "monthly-indicators/$vehicleId"
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

    const val MAP_STATION_PAGE = "map/station/{stationKey}?fuelProduct={fuelProduct}"
    fun mapStationPage(stationKey: String, fuel: FuelProduct): String {
        require(com.anpfuel.domain.discovery.StationPageIdentity.parse(stationKey) != null) { "invalid station identity" }
        return "map/station/$stationKey?fuelProduct=${fuel.name}"
    }

    const val MAP_CHARGING_COMMUNITY = "map/charging/{placeId}"
    fun mapChargingCommunity(placeId: String): String {
        require(java.util.UUID.fromString(placeId).toString() == placeId) { "invalid charging place identity" }
        return "map/charging/$placeId"
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
