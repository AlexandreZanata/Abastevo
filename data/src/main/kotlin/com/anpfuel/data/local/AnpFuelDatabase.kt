package com.anpfuel.data.local

import androidx.room.Database
import androidx.room.RoomDatabase
import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.ImportAuditLogDao
import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.local.dao.MunicipalityFtsDao
import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.local.dao.VehicleDao
import com.anpfuel.data.local.entity.AveragePriceEntity
import com.anpfuel.data.local.entity.ImportAuditLogEntity
import com.anpfuel.data.local.entity.MunicipalityCatalogEntity
import com.anpfuel.data.local.entity.MunicipalityFtsEntity
import com.anpfuel.data.local.entity.StationPriceEntity
import com.anpfuel.data.local.entity.SurveyWeekEntity
import com.anpfuel.data.local.entity.VehicleEntity

@Database(
    entities = [
        SurveyWeekEntity::class,
        AveragePriceEntity::class,
        StationPriceEntity::class,
        ImportAuditLogEntity::class,
        MunicipalityCatalogEntity::class,
        MunicipalityFtsEntity::class,
        VehicleEntity::class,
    ],
    version = 4,
    exportSchema = true,
)
abstract class AnpFuelDatabase : RoomDatabase() {

    abstract fun surveyWeekDao(): SurveyWeekDao

    abstract fun averagePriceDao(): AveragePriceDao

    abstract fun stationPriceDao(): StationPriceDao

    abstract fun importAuditLogDao(): ImportAuditLogDao

    abstract fun municipalityCatalogDao(): MunicipalityCatalogDao

    abstract fun municipalityFtsDao(): MunicipalityFtsDao

    abstract fun vehicleDao(): VehicleDao
}
