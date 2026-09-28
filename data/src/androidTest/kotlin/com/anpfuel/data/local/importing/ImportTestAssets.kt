package com.anpfuel.data.local.importing

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import java.io.File

internal object ImportTestAssets {

    const val SUMMARY_SAMPLE = "resumo_semanal_lpc_2026-06-07_2026-06-13.xlsx"
    const val STATION_SAMPLE = "revendas_lpc_2026-06-07_2026-06-13.xlsx"

    fun resolveSampleFile(context: Context, assetName: String): File {
        val target = File(context.cacheDir, assetName)
        if (target.exists() && target.length() > 0L) {
            return target
        }
        context.assets.open(assetName).use { input ->
            target.outputStream().use { output -> input.copyTo(output) }
        }
        return target
    }

    fun applicationContext(): Context = ApplicationProvider.getApplicationContext()
}
