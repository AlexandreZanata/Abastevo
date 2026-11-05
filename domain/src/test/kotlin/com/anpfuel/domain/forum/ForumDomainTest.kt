package com.anpfuel.domain.forum

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ForumDomainTest {

    @Test
    fun `ForumCategory parses standard wire codes`() {
        assertEquals(ForumCategory.FUELS, ForumCategory.fromWire("FUELS"))
        assertEquals(ForumCategory.MARKET, ForumCategory.fromWire("MARKET"))
        assertEquals(ForumCategory.REGULATION, ForumCategory.fromWire("REGULATION"))
        assertEquals(ForumCategory.CONSUMPTION, ForumCategory.fromWire("CONSUMPTION"))
        assertEquals(ForumCategory.ROADS_MOBILITY, ForumCategory.fromWire("ROADS_MOBILITY"))
        assertEquals(ForumCategory.DRIVER_ECONOMY, ForumCategory.fromWire("DRIVER_ECONOMY"))
        assertEquals(ForumCategory.ABASTEVO_DATA, ForumCategory.fromWire("ABASTEVO_DATA"))
    }

    @Test
    fun `ForumCategory maps unknown or empty wire codes to UNKNOWN`() {
        assertEquals(ForumCategory.UNKNOWN, ForumCategory.fromWire("UNKNOWN_FUTURE_CATEGORY"))
        assertEquals(ForumCategory.UNKNOWN, ForumCategory.fromWire(""))
        assertEquals(ForumCategory.UNKNOWN, ForumCategory.fromWire(null))
        assertEquals(ForumCategory.UNKNOWN, ForumCategory.fromWire("   "))
    }

    @Test
    fun `ForumPostKind parses standard wire codes`() {
        assertEquals(ForumPostKind.EDITORIAL, ForumPostKind.fromWire("EDITORIAL"))
        assertEquals(ForumPostKind.NEWS, ForumPostKind.fromWire("NEWS"))
        assertEquals(ForumPostKind.COMMUNITY, ForumPostKind.fromWire("COMMUNITY"))
        assertEquals(ForumPostKind.TOPIC, ForumPostKind.fromWire("TOPIC"))
    }

    @Test
    fun `ForumPostKind maps unknown or empty wire values to UNKNOWN`() {
        assertEquals(ForumPostKind.UNKNOWN, ForumPostKind.fromWire("POLL"))
        assertEquals(ForumPostKind.UNKNOWN, ForumPostKind.fromWire("VIDEO"))
        assertEquals(ForumPostKind.UNKNOWN, ForumPostKind.fromWire(""))
        assertEquals(ForumPostKind.UNKNOWN, ForumPostKind.fromWire(null))
    }

    @Test
    fun `ForumSafeLinks accepts valid HTTPS URLs`() {
        assertTrue(ForumSafeLinks.isAllowedUrl("https://abastevo.com.br/artigo-1"))
        assertTrue(ForumSafeLinks.isAllowedUrl("https://www.gov.br/anp/pt-br/precos"))
        assertTrue(ForumSafeLinks.isAllowedUrl("https://subdomain.example.org:443/path?query=val#frag"))

        val safe = ForumSafeLinks.requireSafeHttpsUrl("https://abastevo.com.br/artigo-1")
        assertEquals("https://abastevo.com.br/artigo-1", safe)
    }

    @Test
    fun `ForumSafeLinks refuses cleartext HTTP and dangerous schemes`() {
        assertFalse(ForumSafeLinks.isAllowedUrl("http://insecure.example.com"))
        assertFalse(ForumSafeLinks.isAllowedUrl("javascript:alert(1)"))
        assertFalse(ForumSafeLinks.isAllowedUrl("file:///etc/passwd"))
        assertFalse(ForumSafeLinks.isAllowedUrl("content://media/external"))
        assertFalse(ForumSafeLinks.isAllowedUrl("data:text/html,<html>"))

        val ex = assertThrows(ForumException.SecurityViolation::class.java) {
            ForumSafeLinks.requireSafeHttpsUrl("http://insecure.example.com")
        }
        assertEquals("forum.insecure-link", ex.code)
    }

    @Test
    fun `ForumSafeLinks refuses user-info in URLs`() {
        assertFalse(ForumSafeLinks.isAllowedUrl("https://user:pass@evil.com/phish"))
        assertFalse(ForumSafeLinks.isAllowedUrl("https://admin@abastevo.com.br"))

        assertThrows(ForumException.SecurityViolation::class.java) {
            ForumSafeLinks.requireSafeHttpsUrl("https://user:pass@evil.com/phish")
        }
    }

    @Test
    fun `ForumSafeLinks refuses non-standard ports`() {
        assertFalse(ForumSafeLinks.isAllowedUrl("https://abastevo.com.br:8080/artigo"))
        assertFalse(ForumSafeLinks.isAllowedUrl("https://abastevo.com.br:80/artigo"))
    }

    @Test
    fun `ForumSafeLinks refuses empty or oversized URLs`() {
        assertFalse(ForumSafeLinks.isAllowedUrl(null))
        assertFalse(ForumSafeLinks.isAllowedUrl(""))
        assertFalse(ForumSafeLinks.isAllowedUrl("   "))
        assertFalse(ForumSafeLinks.isAllowedUrl("https://" + "a".repeat(2048) + ".com"))
    }

    @Test
    fun `ForumPostSummary and Detail support null provenance and optional fields`() {
        val source = ForumSourceAttribution(
            name = "Abastevo Editorial",
            url = "https://abastevo.com.br/artigo-1",
            attribution = "Redação Abastevo",
            licenseUrl = "https://creativecommons.org/licenses/by/4.0/",
        )
        assertEquals("Abastevo Editorial", source.name)
        assertEquals("https://abastevo.com.br/artigo-1", source.url)
        assertEquals("Redação Abastevo", source.attribution)
        assertEquals("https://creativecommons.org/licenses/by/4.0/", source.licenseUrl)

        val region = ForumRegion(uf = "SP", municipalityIbge = 3550308)
        assertEquals("SP", region.uf)
        assertEquals(3550308, region.municipalityIbge)

        val summary = ForumPostSummary(
            id = "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
            kind = ForumPostKind.EDITORIAL,
            rawKind = "EDITORIAL",
            category = ForumCategory.FUELS,
            rawCategory = "FUELS",
            title = "Titulo",
            excerpt = "Resumo",
            source = source,
            publicAlias = "Alias",
            firstPublishedAt = "2026-11-05T12:00:00Z",
            updatedAt = "2026-11-05T12:00:00Z",
            revision = 1L,
            commentsEnabled = true,
            region = region,
            fuelProduct = "GASOLINA",
            stationId = "11111111-1111-4111-8111-111111111111",
        )
        assertEquals("9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d", summary.id)
        assertEquals(ForumPostKind.EDITORIAL, summary.kind)
        assertEquals("EDITORIAL", summary.rawKind)
        assertEquals(ForumCategory.FUELS, summary.category)
        assertEquals("FUELS", summary.rawCategory)
        assertEquals("Titulo", summary.title)
        assertEquals("Resumo", summary.excerpt)
        assertEquals(source, summary.source)
        assertEquals("Alias", summary.publicAlias)
        assertEquals("2026-11-05T12:00:00Z", summary.firstPublishedAt)
        assertEquals("2026-11-05T12:00:00Z", summary.updatedAt)
        assertEquals(1L, summary.revision)
        assertTrue(summary.commentsEnabled)
        assertEquals(region, summary.region)
        assertEquals("GASOLINA", summary.fuelProduct)
        assertEquals("11111111-1111-4111-8111-111111111111", summary.stationId)
        assertNotNull(summary.toString())
        assertEquals(summary, summary.copy())

        val detail = ForumPostDetail(
            id = summary.id,
            kind = summary.kind,
            rawKind = summary.rawKind,
            category = summary.category,
            rawCategory = summary.rawCategory,
            title = summary.title,
            excerpt = summary.excerpt,
            bodyText = "Corpo completo do artigo editorial.",
            source = source,
            publicAlias = summary.publicAlias,
            sourcePublishedAt = "2026-11-05T11:00:00Z",
            firstPublishedAt = summary.firstPublishedAt,
            updatedAt = summary.updatedAt,
            revision = 1L,
            commentsEnabled = true,
            correction = "Correção informativa",
            region = region,
            fuelProduct = "GASOLINA",
            stationId = "11111111-1111-4111-8111-111111111111",
        )
        assertEquals("Corpo completo do artigo editorial.", detail.bodyText)
        assertEquals("Correção informativa", detail.correction)
        assertEquals("2026-11-05T11:00:00Z", detail.sourcePublishedAt)
        assertNotNull(detail.toString())
        assertEquals(detail, detail.copy())

        val feedPage = ForumFeedPage(
            items = listOf(summary),
            nextCursor = "cursor-next",
            snapshot = "rev-1",
            fetchedAt = "2026-11-05T12:00:00Z",
        )
        assertEquals(1, feedPage.items.size)
        assertEquals("cursor-next", feedPage.nextCursor)
        assertEquals("rev-1", feedPage.snapshot)
        assertEquals("2026-11-05T12:00:00Z", feedPage.fetchedAt)
        assertNotNull(feedPage.toString())
        assertEquals(feedPage, feedPage.copy())

        val catItem = ForumCategoryItem(
            code = ForumCategory.FUELS,
            rawCode = "FUELS",
            name = "Combustíveis",
            description = "Preços e tributação",
        )
        assertEquals(ForumCategory.FUELS, catItem.code)
        assertEquals("FUELS", catItem.rawCode)
        assertEquals("Combustíveis", catItem.name)
        assertEquals("Preços e tributação", catItem.description)
        assertNotNull(catItem.toString())
        assertEquals(catItem, catItem.copy())

        val catResult = ForumCategoriesResult(
            items = listOf(catItem),
            taxonomyVersion = 1,
        )
        assertEquals(1, catResult.items.size)
        assertEquals(1, catResult.taxonomyVersion)
        assertNotNull(catResult.toString())
        assertEquals(catResult, catResult.copy())
    }

    @Test
    fun `ForumException types preserve error codes`() {
        val invalid = ForumException.Invalid("forum.bad-limit")
        assertEquals("forum.bad-limit", invalid.code)

        val notFound = ForumException.NotFound("forum.not-found")
        assertEquals("forum.not-found", notFound.code)

        val disabled = ForumException.Disabled("forum.disabled")
        assertEquals("forum.disabled", disabled.code)

        val sec = ForumException.SecurityViolation("forum.insecure-link")
        assertEquals("forum.insecure-link", sec.code)

        val unavail = ForumException.Unavailable("forum.unavailable")
        assertEquals("forum.unavailable", unavail.code)

        val offline = ForumException.Offline("forum.offline")
        assertEquals("forum.offline", offline.code)
    }

    @Test
    fun `ForumSafeLinks handles host without dot or trailing whitespace`() {
        assertFalse(ForumSafeLinks.isAllowedUrl("https://localhost/path"))
        assertFalse(ForumSafeLinks.isAllowedUrl("https://internal/path"))
        assertTrue(ForumSafeLinks.isAllowedUrl("   https://example.com/path   "))
    }
}
