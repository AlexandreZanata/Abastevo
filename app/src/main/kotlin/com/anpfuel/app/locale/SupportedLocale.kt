package com.anpfuel.app.locale

import android.content.Context
import android.os.LocaleList
import androidx.annotation.VisibleForTesting
import androidx.annotation.DrawableRes
import androidx.annotation.StringRes
import com.anpfuel.app.R
import java.util.Locale

/**
 * UC-008 — App-supported UI locale with a representative country flag.
 *
 * SVG sources: [lipis/flag-icons](https://github.com/lipis/flag-icons) (MIT), see `docs/assets/flags/ATTRIBUTION.md`.
 */
data class SupportedLocale(
    val localeTag: String,
    @DrawableRes val flagResId: Int,
    @StringRes val labelRes: Int,
)

object AppLocales {

    const val ENGLISH_TAG = "en"
    const val PORTUGUESE_BRAZIL_TAG = "pt-BR"
    const val SPANISH_TAG = "es"
    const val FRENCH_TAG = "fr"
    const val CHINESE_TAG = "zh-CN"
    const val RUSSIAN_TAG = "ru"
    const val GERMAN_TAG = "de"
    const val JAPANESE_TAG = "ja"

    val all: List<SupportedLocale> = listOf(
        SupportedLocale(PORTUGUESE_BRAZIL_TAG, R.drawable.ic_flag_br, R.string.a11y_language_portuguese),
        SupportedLocale(ENGLISH_TAG, R.drawable.ic_flag_us, R.string.a11y_language_english),
        SupportedLocale(SPANISH_TAG, R.drawable.ic_flag_es, R.string.a11y_language_spanish),
        SupportedLocale(FRENCH_TAG, R.drawable.ic_flag_fr, R.string.a11y_language_french),
        SupportedLocale(CHINESE_TAG, R.drawable.ic_flag_cn, R.string.a11y_language_chinese),
        SupportedLocale(RUSSIAN_TAG, R.drawable.ic_flag_ru, R.string.a11y_language_russian),
        SupportedLocale(GERMAN_TAG, R.drawable.ic_flag_de, R.string.a11y_language_german),
        SupportedLocale(JAPANESE_TAG, R.drawable.ic_flag_jp, R.string.a11y_language_japanese),
    )

    val supportedLocaleTags: List<String> = all.map { it.localeTag }

    fun find(localeTag: String): SupportedLocale? =
        all.firstOrNull { it.localeTag == localeTag }

    fun resolveDeviceLocaleTag(context: Context): String {
        @Suppress("DEPRECATION")
        val locales = context.resources.configuration.locales
        return resolveFromLocaleList(locales)
    }

    @VisibleForTesting
    internal fun resolveFromLocaleList(locales: LocaleList): String {
        for (index in 0 until locales.size()) {
            matchLocale(locales[index])?.let { return it }
        }
        return PORTUGUESE_BRAZIL_TAG
    }

    @VisibleForTesting
    internal fun matchLocale(locale: Locale): String? {
        val languageTag = locale.toLanguageTag()
        if (languageTag in supportedLocaleTags) {
            return languageTag
        }

        return when (locale.language) {
            "pt" -> PORTUGUESE_BRAZIL_TAG
            "en" -> ENGLISH_TAG
            "es" -> SPANISH_TAG
            "fr" -> FRENCH_TAG
            "zh" -> CHINESE_TAG
            "ru" -> RUSSIAN_TAG
            "de" -> GERMAN_TAG
            "ja" -> JAPANESE_TAG
            else -> null
        }
    }

    fun normalizeLocaleTag(localeTag: String?): String {
        val migrated = when (localeTag) {
            "hi" -> JAPANESE_TAG
            "ar" -> GERMAN_TAG
            "bn" -> RUSSIAN_TAG
            else -> localeTag
        }
        val resolved = migrated?.takeIf { it.isNotBlank() } ?: PORTUGUESE_BRAZIL_TAG
        return if (resolved in supportedLocaleTags) resolved else PORTUGUESE_BRAZIL_TAG
    }
}
