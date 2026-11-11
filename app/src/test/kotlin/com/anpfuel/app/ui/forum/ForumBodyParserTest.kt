package com.anpfuel.app.ui.forum

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class ForumBodyParserTest {

    @Test
    fun `short unpunctuated line is a heading and dash lines are bullets`() {
        val body = "Intro paragraph.\n\nA regra prática dos 70%\n\n- Até 0,70: etanol.\n- Acima: gasolina.\n\nFinal text."
        assertEquals(
            listOf(
                ForumBodyBlock.Paragraph("Intro paragraph."),
                ForumBodyBlock.Heading("A regra prática dos 70%"),
                ForumBodyBlock.Bullets(listOf("Até 0,70: etanol.", "Acima: gasolina.")),
                ForumBodyBlock.Paragraph("Final text."),
            ),
            parseForumBody(body),
        )
    }

    @Test
    fun `punctuated or multi-line blocks stay paragraphs`() {
        val body = "Ends with colon:\n\nLine one\nline two\n\n" + "x".repeat(90)
        assertEquals(
            listOf(
                ForumBodyBlock.Paragraph("Ends with colon:"),
                ForumBodyBlock.Paragraph("Line one\nline two"),
                ForumBodyBlock.Paragraph("x".repeat(90)),
            ),
            parseForumBody(body),
        )
    }

    @Test
    fun `mixed list and text block is kept as a paragraph`() {
        val body = "Steps:\n- one\nnot a bullet"
        assertEquals(listOf(ForumBodyBlock.Paragraph(body)), parseForumBody(body))
    }
}
