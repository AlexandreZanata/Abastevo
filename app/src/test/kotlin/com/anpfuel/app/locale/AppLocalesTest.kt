package com.anpfuel.app.locale

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test
import java.util.Locale

class AppLocalesTest {

    @Test
    fun exposesEightLocalesInGridOrder() {
        assertEquals(8, AppLocales.all.size)
        assertEquals(
            listOf("pt-BR", "en", "es", "fr", "zh-CN", "ru", "de", "ja"),
            AppLocales.supportedLocaleTags,
        )
    }

    @Test
    fun migratesRetiredLocaleTags() {
        assertEquals("ja", AppLocales.normalizeLocaleTag("hi"))
        assertEquals("de", AppLocales.normalizeLocaleTag("ar"))
        assertEquals("ru", AppLocales.normalizeLocaleTag("bn"))
    }

    @Test
    fun fallsBackToPortugueseBrazilForUnsupportedLocaleTag() {
        assertEquals(AppLocales.PORTUGUESE_BRAZIL_TAG, AppLocales.normalizeLocaleTag("it"))
        assertEquals(AppLocales.PORTUGUESE_BRAZIL_TAG, AppLocales.normalizeLocaleTag(null))
    }

    @Test
    fun matchesExactAndLanguageOnlySupportedLocales() {
        assertEquals(AppLocales.PORTUGUESE_BRAZIL_TAG, AppLocales.matchLocale(Locale.forLanguageTag("pt-BR")))
        assertEquals(AppLocales.PORTUGUESE_BRAZIL_TAG, AppLocales.matchLocale(Locale.forLanguageTag("pt")))
        assertEquals(AppLocales.ENGLISH_TAG, AppLocales.matchLocale(Locale.forLanguageTag("en-US")))
        assertEquals(AppLocales.CHINESE_TAG, AppLocales.matchLocale(Locale.forLanguageTag("zh")))
        assertNull(AppLocales.matchLocale(Locale.forLanguageTag("it-IT")))
    }
}
