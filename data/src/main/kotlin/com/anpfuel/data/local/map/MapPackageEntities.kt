package com.anpfuel.data.local.map

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.ForeignKey
import androidx.room.Index
import androidx.room.PrimaryKey

@Entity(tableName = "map_generation")
data class MapGenerationEntity(
    @PrimaryKey val generation: String,
    @ColumnInfo(name = "manifest_sha256") val manifestSha256: String,
    val active: Boolean,
    @ColumnInfo(name = "activated_at_millis") val activatedAtMillis: Long?,
    /**
     * Native data version (`engine.format_version`, numeric directory name).
     * Empty for rows imported before native-layout publication; those rows
     * are treated as unpublished and require re-activation.
     */
    @ColumnInfo(name = "data_version", defaultValue = "") val dataVersion: String = "",
)

@Entity(
    tableName = "map_package",
    primaryKeys = ["generation", "package_id"],
    foreignKeys = [
        ForeignKey(
            entity = MapGenerationEntity::class,
            parentColumns = ["generation"],
            childColumns = ["generation"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
    indices = [Index("generation"), Index("status")],
)
data class MapPackageEntity(
    @ColumnInfo(name = "package_id") val packageId: String,
    val generation: String,
    val role: String,
    @ColumnInfo(name = "file_name") val fileName: String,
    val bytes: Long,
    val sha256: String,
    val etag: String?,
    val status: String,
    @ColumnInfo(name = "downloaded_bytes") val downloadedBytes: Long,
    val error: String?,
    @ColumnInfo(name = "updated_at_millis") val updatedAtMillis: Long,
)
