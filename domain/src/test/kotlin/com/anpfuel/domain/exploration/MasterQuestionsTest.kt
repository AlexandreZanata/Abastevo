package com.anpfuel.domain.exploration

import java.time.LocalDate
import java.time.ZoneOffset
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

private val testNow = LocalDate.of(2026, 10, 22).atStartOfDay().toInstant(ZoneOffset.UTC)

class MasterQuestionsTest {

    @Test
    fun `liquid paraphrases converge without EV stealing`() {
        for (question in listOf(
            "Qual o maior preço observado da gasolina comum no Brasil?",
            "Onde está o maior preço observado da gasolina comum no país?",
        )) {
            val got = MasterQuestions.interpret(question, testNow)
            assertEquals(InterpretationState.RESOLVED, got.state, question)
            val query = got.query as PriceQuery
            assertEquals("GASOLINE_REGULAR", query.product.fuel)
            assertEquals("DESC", query.metric.sort)
        }
        val stolen = MasterQuestions.interpret("Qual o maior preço observado da gasolina comum no Brasil?", testNow)
        assertTrue(stolen.query !is ChargingPlaceQuery && stolen.query !is EnergyPriceQuery)
    }

    @Test
    fun `accents UF spellings and unknown fuels behave`() {
        val sp = MasterQuestions.interpret("Maior preço observado do etanol em São Paulo", testNow)
        assertEquals("SP", (sp.query as PriceQuery).geography.uf)
        val unknown = MasterQuestions.interpret("Qual o preço do querosene?", testNow)
        assertEquals(InterpretationState.UNSUPPORTED, unknown.state)
    }

    @Test
    fun `ev place paraphrases converge with pinned source`() {
        for (question in listOf(
            "Onde posso carregar com conector CCS2?",
            "Me mostre eletropostos com CCS 2",
        )) {
            val got = MasterQuestions.interpret(question, testNow)
            assertEquals(InterpretationState.RESOLVED, got.state, question)
            val query = got.query as ChargingPlaceQuery
            assertEquals("CCS2", query.connector)
            assertEquals("ocm-br-20261021-v1", query.profile)
            assertTrue(query.page.limit in 1..100)
        }
    }

    @Test
    fun `ev ambiguity asks with concrete alternatives`() {
        val missing = MasterQuestions.interpret("Onde posso carregar meu carro?", testNow)
        assertEquals(InterpretationState.NEEDS_CHOICE, missing.state)
        assertEquals(4, missing.alternatives.size)
        val metric = MasterQuestions.interpret("Qual a recarga mais barata?", testNow)
        assertEquals(InterpretationState.NEEDS_CHOICE, metric.state)
        assertEquals(2, metric.alternatives.size)
    }

    @Test
    fun `scenario keeps declared dimensions and discloses missing`() {
        val got = MasterQuestions.interpret("Quanto custa carregar 50 kWh com 2 horas de estacionamento?", testNow)
        assertEquals(InterpretationState.RESOLVED, got.state)
        val query = got.query as ScenarioCostQuery
        assertTrue(query.dimensions.contains("ENERGY_WH") && query.dimensions.contains("PARKING_SECOND"))
        assertTrue(query.missing.isNotEmpty())
    }

    @Test
    fun `adversarial input never executes`() {
        val got = MasterQuestions.interpret("'; DROP TABLE charging; --", testNow)
        assertEquals(InterpretationState.UNSUPPORTED, got.state)
    }

    private fun assertTrue(value: Boolean, message: String = "") = org.junit.jupiter.api.Assertions.assertTrue(value, message)
}
