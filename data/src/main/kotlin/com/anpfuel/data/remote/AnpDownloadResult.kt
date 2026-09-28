package com.anpfuel.data.remote

import java.io.File

data class AnpDownloadResult(
    val file: File,
    val sourceUrl: String,
    val contentType: String?,
    val sizeBytes: Long,
    val sha256: String,
)
