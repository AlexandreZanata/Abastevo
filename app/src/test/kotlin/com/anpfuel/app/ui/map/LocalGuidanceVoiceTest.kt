package com.anpfuel.app.ui.map

import android.content.Context
import android.os.Handler
import android.speech.tts.TextToSpeech
import android.speech.tts.Voice
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import java.util.Locale
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class LocalGuidanceVoiceTest {
    private val tasks = ArrayDeque<Runnable>()
    private var timeout: Runnable? = null
    private val handler = mockk<Handler> {
        every { post(any()) } answers { tasks.addLast(firstArg()); true }
        every { postDelayed(any(), any<Long>()) } answers { timeout = firstArg(); true }
        every { removeCallbacks(any()) } answers { if (timeout == firstArg<Runnable>()) timeout = null }
    }
    private val speech = mockk<TextToSpeech>(relaxed = true)
    private lateinit var listener: TextToSpeech.OnInitListener
    private val voice = LocalGuidanceVoice(mockk<Context>(relaxed = true), handler) { onInit, _ ->
        listener = onInit
        speech
    }

    @Test fun `pending initialization returns immediately and late completion after timeout is ignored`() {
        val states = mutableListOf<Boolean>()
        voice.start("pt", onAvailability = states::add)
        assertTrue(states.isEmpty())
        assertFalse(voice.available)

        checkNotNull(timeout).run()
        listener.onInit(TextToSpeech.SUCCESS)
        tasks.removeFirst().run()

        assertEquals(listOf(false), states)
        verify(exactly = 1) { speech.shutdown() }
        verify(exactly = 0) { speech.setVoice(any()) }
    }

    @Test fun `a stopped session cannot complete or affect a replacement`() {
        val oldStates = mutableListOf<Boolean>()
        val newStates = mutableListOf<Boolean>()
        voice.start("pt", onAvailability = oldStates::add)
        val oldListener = listener
        voice.stop()
        voice.start("pt", onAvailability = newStates::add)

        oldListener.onInit(TextToSpeech.SUCCESS)
        tasks.removeFirst().run()
        assertTrue(oldStates.isEmpty())
        assertTrue(newStates.isEmpty())

        listener.onInit(TextToSpeech.ERROR)
        tasks.removeFirst().run()
        assertEquals(listOf(false), newStates)
        assertFalse(voice.available)
        verify(exactly = 2) { speech.shutdown() }
    }

    @Test fun `initialization selects an installed offline voice explicitly`() {
        val local = candidateVoice(requiresNetwork = false)
        val network = candidateVoice(requiresNetwork = true)
        val download = candidateVoice(requiresNetwork = false, features = setOf(TextToSpeech.Engine.KEY_FEATURE_NOT_INSTALLED))
        every { speech.voices } returns linkedSetOf(network, download, local)
        every { speech.setVoice(local) } returns TextToSpeech.SUCCESS
        val states = mutableListOf<Boolean>()

        voice.start("pt", onAvailability = states::add)
        listener.onInit(TextToSpeech.SUCCESS)
        tasks.removeFirst().run()

        assertEquals(listOf(true), states)
        assertTrue(voice.available)
        assertEquals(null, timeout)
        verify(exactly = 1) { speech.setVoice(local) }
        verify(exactly = 0) { speech.setVoice(network) }
        verify(exactly = 0) { speech.setVoice(download) }
    }

    private fun candidateVoice(requiresNetwork: Boolean, features: Set<String> = emptySet()): Voice = mockk {
        every { isNetworkConnectionRequired } returns requiresNetwork
        every { this@mockk.features } returns features
        every { locale } returns Locale.forLanguageTag("pt-BR")
    }
}
