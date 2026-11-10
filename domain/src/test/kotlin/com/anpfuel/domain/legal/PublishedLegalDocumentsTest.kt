package com.anpfuel.domain.legal

import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** The accepted versions must be the ones printed on the public pages (B-BR-TA03). */
class PublishedLegalDocumentsTest {
    private fun page(path: String): String {
        val file = generateSequence(File("").absoluteFile) { it.parentFile }
            .map { File(it, path) }.firstOrNull { it.isFile }
        assertTrue(file != null, "public document not found: $path")
        return file!!.readText()
    }

    @Test fun `terms version is the month printed on the public terms page`() {
        val months = listOf("Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho", "Julho", "Agosto",
            "Setembro", "Outubro", "Novembro", "Dezembro")
        val match = Regex("Última atualização:</strong>\\s*([A-Za-zç]+) de ([0-9]{4})")
            .find(page("landing/static/termos/index.html"))
        assertTrue(match != null, "terms page lost its update line")
        val (month, year) = match!!.destructured
        val printed = "%s-%02d".format(year, months.indexOf(month) + 1)
        assertEquals(printed, PublishedLegalDocuments.CURRENT.termsVersion)
    }

    @Test fun `privacy version is the one printed on the public policy page`() {
        val match = Regex("<strong>Versão ([0-9.]+)</strong>")
            .find(page("landing/static/privacidade/index.html"))
        assertTrue(match != null, "policy page lost its version line")
        assertEquals(match!!.groupValues[1], PublishedLegalDocuments.CURRENT.privacyVersion)
    }
}
