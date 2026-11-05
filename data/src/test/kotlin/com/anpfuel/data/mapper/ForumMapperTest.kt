package com.anpfuel.data.mapper

import com.anpfuel.data.local.forum.ForumCacheCodec
import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumCategoryItem
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostDetail
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.forum.ForumPostSummary
import com.anpfuel.domain.forum.ForumRegion
import com.anpfuel.domain.forum.ForumSourceAttribution
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ForumMapperTest {

    @Test
    fun `maps categories json response`() {
        val raw = """
            {
                "items": [
                    {
                        "code": "FUELS",
                        "name": "Combustíveis",
                        "description": "Preços e tributação"
                    },
                    {
                        "code": "UNKNOWN_FUTURE",
                        "name": "Futuro",
                        "description": "Categoria futura"
                    }
                ],
                "taxonomy_version": 2
            }
        """.trimIndent()
        val result = ForumMapper.categories(JSONObject(raw))
        assertEquals(2, result.taxonomyVersion)
        assertEquals(2, result.items.size)
        assertEquals(ForumCategory.FUELS, result.items[0].code)
        assertEquals("FUELS", result.items[0].rawCode)
        assertEquals("Combustíveis", result.items[0].name)
        assertEquals(ForumCategory.UNKNOWN, result.items[1].code)
        assertEquals("UNKNOWN_FUTURE", result.items[1].rawCode)
    }

    @Test
    fun `maps feed json response with full fields`() {
        val raw = """
            {
                "items": [
                    {
                        "id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
                        "kind": "EDITORIAL",
                        "category": "FUELS",
                        "title": "Entenda a formação de preços",
                        "excerpt": "Guia explicativo sobre composição tributária.",
                        "source": {
                            "name": "Abastevo Editorial",
                            "url": "https://abastevo.com.br/artigo-1",
                            "attribution": "Redação Abastevo",
                            "license_url": "https://creativecommons.org/licenses/by/4.0/"
                        },
                        "public_alias": "Equipe Editorial",
                        "first_published_at": "2026-11-05T12:00:00Z",
                        "updated_at": "2026-11-05T12:00:00Z",
                        "revision": 3,
                        "comments_enabled": true,
                        "region": {
                            "uf": "SP",
                            "municipality_ibge": 3550308
                        },
                        "fuel_product": "GASOLINA",
                        "station_id": "11111111-1111-4111-8111-111111111111"
                    }
                ],
                "next_cursor": "cursor-token-abc",
                "snapshot": "rev-5",
                "fetched_at": "2026-11-05T12:00:00Z"
            }
        """.trimIndent()
        val feed = ForumMapper.feed(JSONObject(raw))
        assertEquals("cursor-token-abc", feed.nextCursor)
        assertEquals("rev-5", feed.snapshot)
        assertEquals(1, feed.items.size)

        val item = feed.items[0]
        assertEquals("9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d", item.id)
        assertEquals(ForumPostKind.EDITORIAL, item.kind)
        assertEquals(ForumCategory.FUELS, item.category)
        assertEquals("Entenda a formação de preços", item.title)
        assertNotNull(item.source)
        assertEquals("Abastevo Editorial", item.source?.name)
        assertEquals("https://creativecommons.org/licenses/by/4.0/", item.source?.licenseUrl)
        assertEquals("Equipe Editorial", item.publicAlias)
        assertEquals(3L, item.revision)
        assertEquals("SP", item.region?.uf)
        assertEquals(3550308, item.region?.municipalityIbge)
        assertEquals("GASOLINA", item.fuelProduct)
        assertEquals("11111111-1111-4111-8111-111111111111", item.stationId)
    }

    @Test
    fun `maps post detail with body text and correction`() {
        val raw = """
            {
                "id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
                "kind": "EDITORIAL",
                "category": "FUELS",
                "title": "Entenda a formação de preços",
                "excerpt": "Guia explicativo.",
                "body_text": "Corpo completo do artigo editorial em markdown.",
                "correction": "Nota de correção: valor de alíquota atualizado.",
                "source_published_at": "2026-11-05T10:00:00Z",
                "first_published_at": "2026-11-05T12:00:00Z",
                "updated_at": "2026-11-05T12:30:00Z",
                "revision": 2,
                "comments_enabled": false
            }
        """.trimIndent()
        val detail = ForumMapper.detail(JSONObject(raw))
        assertEquals("Corpo completo do artigo editorial em markdown.", detail.bodyText)
        assertEquals("Nota de correção: valor de alíquota atualizado.", detail.correction)
        assertEquals("2026-11-05T10:00:00Z", detail.sourcePublishedAt)
        assertNull(detail.source)
        assertNull(detail.region)
        assertEquals(false, detail.commentsEnabled)
    }

    @Test
    fun `ForumCacheCodec roundtrip serialization`() {
        // Categories roundtrip
        val catResult = ForumCategoriesResult(
            items = listOf(
                ForumCategoryItem(ForumCategory.FUELS, "FUELS", "Combustíveis", "Preços"),
            ),
            taxonomyVersion = 1,
        )
        val catEncoded = ForumCacheCodec.encodeCategories(catResult)
        val catDecoded = ForumCacheCodec.decodeCategories(catEncoded)
        assertEquals(catResult, catDecoded)

        // Feed roundtrip
        val source = ForumSourceAttribution("Fonte", "https://fonte.com", "Redação", null)
        val summary = ForumPostSummary(
            id = "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
            kind = ForumPostKind.EDITORIAL,
            rawKind = "EDITORIAL",
            category = ForumCategory.FUELS,
            rawCategory = "FUELS",
            title = "Título",
            excerpt = "Resumo",
            source = source,
            publicAlias = null,
            firstPublishedAt = "2026-11-05T12:00:00Z",
            updatedAt = "2026-11-05T12:00:00Z",
            revision = 1L,
            commentsEnabled = true,
            region = ForumRegion("RJ", 3304557),
            fuelProduct = "DIESEL",
            stationId = null,
        )
        val feedPage = ForumFeedPage(listOf(summary), "cursor-next", "rev-2", "2026-11-05T12:00:00Z")
        val feedEncoded = ForumCacheCodec.encodeFeed(feedPage)
        val feedDecoded = ForumCacheCodec.decodeFeed(feedEncoded)
        assertEquals(feedPage.items.size, feedDecoded.items.size)
        assertEquals(feedPage.nextCursor, feedDecoded.nextCursor)
        assertEquals(feedPage.snapshot, feedDecoded.snapshot)
        assertEquals(feedPage.items[0].id, feedDecoded.items[0].id)
        assertEquals(feedPage.items[0].region, feedDecoded.items[0].region)

        // Detail roundtrip
        val detail = ForumPostDetail(
            id = summary.id,
            kind = summary.kind,
            rawKind = summary.rawKind,
            category = summary.category,
            rawCategory = summary.rawCategory,
            title = summary.title,
            excerpt = summary.excerpt,
            bodyText = "Corpo completo detalhado.",
            source = source,
            publicAlias = "Alias",
            sourcePublishedAt = "2026-11-05T10:00:00Z",
            firstPublishedAt = summary.firstPublishedAt,
            updatedAt = summary.updatedAt,
            revision = 2L,
            commentsEnabled = true,
            correction = "Correção",
            region = summary.region,
            fuelProduct = "DIESEL",
            stationId = null,
        )
        val detailEncoded = ForumCacheCodec.encodeDetail(detail)
        val detailDecoded = ForumCacheCodec.decodeDetail(detailEncoded)
        assertEquals(detail.id, detailDecoded.id)
        assertEquals(detail.bodyText, detailDecoded.bodyText)
        assertEquals(detail.correction, detailDecoded.correction)
        assertEquals(detail.sourcePublishedAt, detailDecoded.sourcePublishedAt)
        assertEquals(detail.source, detailDecoded.source)
        assertEquals(detail.region, detailDecoded.region)
    }
}
