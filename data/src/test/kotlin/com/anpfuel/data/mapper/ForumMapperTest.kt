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
    fun `maps bulletin provenance and correction link`() {
        val raw = """
            {
                "id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
                "kind": "BULLETIN",
                "category": "ABASTEVO_DATA",
                "title": "Gasolina comum em SP: R$ 6,018/L",
                "excerpt": "Média amostral de 100 postos.",
                "body_text": "Corpo determinístico.",
                "first_published_at": "2026-11-05T12:00:00Z",
                "updated_at": "2026-11-05T12:00:00Z",
                "revision": 1,
                "comments_enabled": true,
                "supersedes_post_id": "00000000-0000-4000-8000-000000000009",
                "bulletin": {
                    "methodology_version": "forum-official-mean-v1",
                    "template_version": "forum-bulletin-v1",
                    "fuel": "GASOLINE_REGULAR",
                    "unit": "L",
                    "current": {
                        "revision_id": "11111111-1111-4111-8111-111111111111",
                        "survey_start": "2026-10-26",
                        "survey_end": "2026-11-01",
                        "published_at": "2026-11-02T12:00:00Z",
                        "source_url": "https://www.gov.br/anp/a.xlsx",
                        "source_checksum": "sha256:a",
                        "parser_version": "v1",
                        "sum_milli_brl": 601800,
                        "observation_count": 100,
                        "sampled_stations": 100,
                        "source_observations": 105,
                        "source_stations": 120,
                        "mean_numerator": 6018,
                        "mean_denominator": 1,
                        "mean": "6018"
                    },
                    "previous": {
                        "revision_id": "22222222-2222-4222-8222-222222222222",
                        "survey_start": "2026-10-19",
                        "survey_end": "2026-10-25",
                        "published_at": "2026-10-27T12:00:00Z",
                        "source_url": "https://www.gov.br/anp/b.xlsx",
                        "source_checksum": "sha256:b",
                        "parser_version": "v1",
                        "sum_milli_brl": 590000,
                        "observation_count": 100,
                        "sampled_stations": 100,
                        "source_observations": 100,
                        "source_stations": 118,
                        "mean_numerator": 5900,
                        "mean_denominator": 1,
                        "mean": "5900"
                    }
                }
            }
        """.trimIndent()
        val detail = ForumMapper.detail(JSONObject(raw))
        assertEquals(ForumPostKind.BULLETIN, detail.kind)
        assertEquals("forum-official-mean-v1", detail.bulletin?.methodologyVersion)
        assertEquals("GASOLINE_REGULAR", detail.bulletin?.fuel)
        assertEquals(601800L, detail.bulletin?.current?.sumMilliBrl)
        assertEquals(120L, detail.bulletin?.current?.sourceStations)
        assertEquals(5900L, detail.bulletin?.previous?.meanNumerator)
        assertEquals("00000000-0000-4000-8000-000000000009", detail.supersedesPostId)
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

    @Test
    fun `maps comments page json response`() {
        val raw = """
            {
                "items": [
                    {
                        "id": "11111111-1111-4111-8111-111111111111",
                        "post_id": "22222222-2222-4222-8222-222222222222",
                        "parent_id": null,
                        "depth": 0,
                        "sequence": 1,
                        "revision": 3,
                        "public_alias": "Motorista SP",
                        "body_text": "Ótima explicação sobre tributos.",
                        "created_at": "2026-11-05T14:00:00Z",
                        "updated_at": "2026-11-05T14:00:00Z",
                        "reply_count": 2
                    }
                ],
                "next_cursor": "cursor-comments-abc"
            }
        """.trimIndent()
        val result = ForumMapper.comments(JSONObject(raw))
        assertEquals(1, result.items.size)
        assertEquals("cursor-comments-abc", result.nextCursor)

        val item = result.items[0]
        assertEquals("11111111-1111-4111-8111-111111111111", item.id)
        assertEquals("22222222-2222-4222-8222-222222222222", item.postId)
        assertNull(item.parentId)
        assertEquals("Motorista SP", item.publicAlias)
        assertEquals("Ótima explicação sobre tributos.", item.bodyText)
        assertEquals(1L, item.sequence)
        assertEquals(3L, item.revision)
        assertEquals(2, item.replyCount)
    }

    @Test
    fun `maps replies page json response`() {
        val raw = """
            {
                "items": [
                    {
                        "id": "33333333-3333-4333-8333-333333333333",
                        "post_id": "22222222-2222-4222-8222-222222222222",
                        "parent_id": "11111111-1111-4111-8111-111111111111",
                        "depth": 1,
                        "sequence": 2,
                        "public_alias": "Motorista RJ",
                        "body_text": "Concordo plenamente.",
                        "created_at": "2026-11-05T14:10:00Z",
                        "updated_at": "2026-11-05T14:10:00Z",
                        "reply_count": 0
                    }
                ],
                "next_cursor": null
            }
        """.trimIndent()
        val result = ForumMapper.replies(JSONObject(raw))
        assertEquals(1, result.items.size)
        assertNull(result.nextCursor)
        assertEquals("33333333-3333-4333-8333-333333333333", result.items[0].id)
        assertEquals("11111111-1111-4111-8111-111111111111", result.items[0].parentId)
    }

    @Test
    fun `maps comment receipt json response`() {
        val raw = """
            {
                "id": "11111111-1111-4111-8111-111111111111",
                "post_id": "22222222-2222-4222-8222-222222222222",
                "parent_id": null,
                "depth": 0,
                "sequence": 1,
                "revision": 1,
                "publication_state": "PENDING_REVIEW",
                "visibility": "VISIBLE",
                "body_text": "Comentário enviado para moderação antes da publicação.",
                "created_at": "2026-11-05T14:00:00Z",
                "updated_at": "2026-11-05T14:00:00Z"
            }
        """.trimIndent()
        val receipt = ForumMapper.commentReceipt(JSONObject(raw))
        assertEquals("11111111-1111-4111-8111-111111111111", receipt.id)
        assertEquals(1L, receipt.revision)
        assertEquals("PENDING_REVIEW", receipt.publicationState)
        assertEquals("VISIBLE", receipt.visibility)
        assertEquals("Comentário enviado para moderação antes da publicação.", receipt.bodyText)
    }

    @Test
    fun `maps report receipt json response`() {
        val raw = """
            {
                "report_id": "44444444-4444-4444-8444-444444444444",
                "case_id": "case-555",
                "target_type": "FORUM_COMMENT",
                "target_id": "11111111-1111-4111-8111-111111111111",
                "reason": "SPAM",
                "status": "OPEN",
                "created_at": "2026-11-05T14:20:00Z"
            }
        """.trimIndent()
        val receipt = ForumMapper.reportReceipt(JSONObject(raw))
        assertEquals("44444444-4444-4444-8444-444444444444", receipt.reportId)
        assertEquals("case-555", receipt.caseId)
        assertEquals("FORUM_COMMENT", receipt.targetType)
        assertEquals("SPAM", receipt.reason)
        assertEquals("OPEN", receipt.status)
    }

    @Test
    fun `maps topic receipt json response direct and enveloped`() {
        val rawDirect = """
            {
                "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
                "kind": "TOPIC",
                "category": "FUELS",
                "title": "Gasolina em SP",
                "body_text": "Discussão de preços",
                "revision": 2,
                "publication_state": "PENDING_REVIEW",
                "visibility": "VISIBLE",
                "region": {
                    "uf": "SP",
                    "municipality_ibge": 3550308
                },
                "fuel_product": "GASOLINE_REGULAR",
                "station_id": "550e8400-e29b-41d4-a716-446655440000",
                "public_alias": "Alex",
                "created_at": "2026-11-06T10:00:00Z",
                "updated_at": "2026-11-06T10:30:00Z",
                "published_at": null,
                "replayed": true
            }
        """.trimIndent()
        val direct = ForumMapper.topicReceipt(JSONObject(rawDirect))
        assertEquals("3fa85f64-5717-4562-b3fc-2c963f66afa6", direct.id)
        assertEquals("TOPIC", direct.kind)
        assertEquals(ForumCategory.FUELS, direct.category)
        assertEquals("Gasolina em SP", direct.title)
        assertEquals("Discussão de preços", direct.bodyText)
        assertEquals(2L, direct.revision)
        assertEquals("PENDING_REVIEW", direct.publicationState)
        assertEquals("VISIBLE", direct.visibility)
        assertEquals("SP", direct.region?.uf)
        assertEquals(3550308, direct.region?.municipalityIbge)
        assertEquals("GASOLINE_REGULAR", direct.fuelProduct)
        assertEquals("550e8400-e29b-41d4-a716-446655440000", direct.stationId)
        assertEquals("Alex", direct.publicAlias)
        assertTrue(direct.replayed)

        // Enveloped in "topic"
        val rawEnveloped = """
            {
                "topic": $rawDirect
            }
        """.trimIndent()
        val enveloped = ForumMapper.topicReceipt(JSONObject(rawEnveloped))
        assertEquals(direct.id, enveloped.id)
        assertEquals(direct.title, enveloped.title)
        assertTrue(enveloped.replayed)
    }
}
