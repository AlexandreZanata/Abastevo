package com.anpfuel.app.ui.map

import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFocusRequest
import android.media.AudioManager
import android.os.Build
import android.os.Bundle
import android.speech.tts.TextToSpeech
import android.speech.tts.UtteranceProgressListener
import android.speech.tts.Voice
import java.util.Locale
import java.util.UUID
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

/**
 * Local Android TTS only, owned explicitly during foreground guidance.
 * Accepts only an installed local voice that does not require network;
 * network/download-required voices refuse and guidance stays textual.
 * Uses navigation audio focus, stops on focus loss/calls and fences stale
 * engine callbacks by generation.
 */
class LocalGuidanceVoice(private val appContext: Context) {
    private var tts: TextToSpeech? = null
    private var enginePackage: String? = null
    private var generation = 0L
    private var audioManager: AudioManager? = null
    private var focusRequest: AudioFocusRequest? = null
    private var focusHeld = false

    @Volatile var available: Boolean = false
        private set

    private val focusListener = AudioManager.OnAudioFocusChangeListener { change ->
        if (change == AudioManager.AUDIOFOCUS_LOSS || change == AudioManager.AUDIOFOCUS_LOSS_TRANSIENT) {
            runCatching { tts?.stop() }
        }
    }

    /** Map app language tags to a concrete local TTS locale; unknown stays null. */
    fun ttsLocaleFor(language: String): Locale? {
        val tag = language.trim().replace('_', '-')
        if (tag.isEmpty()) return null
        return when {
            tag.equals("pt-BR", ignoreCase = true) -> Locale("pt", "BR")
            tag.equals("pt", ignoreCase = true) -> Locale("pt", "BR")
            tag.equals("zh-Hans", ignoreCase = true) -> Locale.SIMPLIFIED_CHINESE
            tag.equals("zh-CN", ignoreCase = true) -> Locale.SIMPLIFIED_CHINESE
            tag.equals("zh-SG", ignoreCase = true) -> Locale.SIMPLIFIED_CHINESE
            tag.equals("zh", ignoreCase = true) -> Locale.SIMPLIFIED_CHINESE
            tag.equals("en", ignoreCase = true) -> Locale.US
            tag.equals("de", ignoreCase = true) -> Locale.GERMANY
            tag.equals("es", ignoreCase = true) -> Locale("es", "ES")
            tag.equals("fr", ignoreCase = true) -> Locale.FRANCE
            tag.equals("ja", ignoreCase = true) -> Locale.JAPAN
            tag.equals("ru", ignoreCase = true) -> Locale("ru", "RU")
            else -> null
        }
    }

    fun start(language: String, preferredEngine: String? = null): Boolean {
        stop()
        val token = ++generation
        val target = ttsLocaleFor(language) ?: return false
        val latch = CountDownLatch(1)
        val statusRef = AtomicReference<Int>(TextToSpeech.ERROR)
        return try {
            val instance = if (!preferredEngine.isNullOrEmpty()) {
                enginePackage = preferredEngine
                TextToSpeech(appContext, { status ->
                    statusRef.set(status)
                    latch.countDown()
                }, preferredEngine)
            } else {
                enginePackage = null
                TextToSpeech(appContext, { status ->
                    statusRef.set(status)
                    latch.countDown()
                })
            }
            // Bound init wait; a hung engine must not block guidance start.
            latch.await(8, TimeUnit.SECONDS)
            if (token != generation) {
                runCatching { instance.shutdown() }
                return false
            }
            if (statusRef.get() != TextToSpeech.SUCCESS) {
                runCatching { instance.shutdown() }
                return false
            }
            tts = instance
            instance.setOnUtteranceProgressListener(object : UtteranceProgressListener() {
                override fun onStart(utteranceId: String?) = Unit
                override fun onDone(utteranceId: String?) = Unit
                @Deprecated("Deprecated in Java")
                override fun onError(utteranceId: String?) = Unit
                override fun onError(utteranceId: String?, errorCode: Int) = Unit
            })
            // Accept only an installed local voice that needs no network.
            val setStatus = instance.setLanguage(target)
            if (setStatus == TextToSpeech.LANG_MISSING_DATA || setStatus == TextToSpeech.LANG_NOT_SUPPORTED) {
                runCatching { instance.shutdown() }
                tts = null
                return false
            }
            if (!hasLocalVoice(instance, target)) {
                runCatching { instance.shutdown() }
                tts = null
                return false
            }
            instance.setSpeechRate(1.0f)
            audioManager = appContext.getSystemService(AudioManager::class.java)
            available = true
            true
        } catch (_: Exception) {
            runCatching { tts?.shutdown() }
            tts = null
            available = false
            false
        }
    }

    private fun hasLocalVoice(instance: TextToSpeech, target: Locale): Boolean {
        return try {
            val voices: Set<Voice> = instance.voices ?: return true
            voices.any { voice ->
                !voice.isNetworkConnectionRequired &&
                    voice.locale.language == target.language &&
                    (voice.locale.country.isEmpty() || target.country.isEmpty() || voice.locale.country == target.country)
            }
        } catch (_: Exception) {
            false
        }
    }

    fun speak(text: String): Boolean {
        val instance = tts ?: return false
        if (!available || text.isBlank()) return false
        return try {
            if (!requestFocus()) return false
            val params = Bundle()
            val id = UUID.randomUUID().toString()
            val status = instance.speak(text, TextToSpeech.QUEUE_ADD, params, id)
            status == TextToSpeech.SUCCESS
        } catch (_: Exception) {
            false
        }
    }

    fun pause() {
        runCatching { tts?.stop() }
        abandonFocus()
    }

    fun stop() {
        generation++
        runCatching { tts?.stop() }
        runCatching { tts?.shutdown() }
        tts = null
        available = false
        abandonFocus()
    }

    private fun requestFocus(): Boolean {
        val manager = audioManager ?: appContext.getSystemService(AudioManager::class.java)?.also { audioManager = it }
            ?: return false
        return try {
            if (Build.VERSION.SDK_INT >= 26) {
                val request = focusRequest ?: AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN_TRANSIENT_MAY_DUCK)
                    .setAudioAttributes(
                        AudioAttributes.Builder()
                            .setUsage(AudioAttributes.USAGE_ASSISTANCE_NAVIGATION_GUIDANCE)
                            .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                            .build(),
                    )
                    .setOnAudioFocusChangeListener(focusListener)
                    .build().also { focusRequest = it }
                val result = manager.requestAudioFocus(request)
                focusHeld = result == AudioManager.AUDIOFOCUS_REQUEST_GRANTED
                focusHeld
            } else {
                @Suppress("DEPRECATION")
                val result = manager.requestAudioFocus(
                    focusListener,
                    AudioManager.STREAM_MUSIC,
                    AudioManager.AUDIOFOCUS_GAIN_TRANSIENT_MAY_DUCK,
                )
                focusHeld = result == AudioManager.AUDIOFOCUS_REQUEST_GRANTED
                focusHeld
            }
        } catch (_: Exception) {
            false
        }
    }

    private fun abandonFocus() {
        if (!focusHeld) return
        focusHeld = false
        try {
            val manager = audioManager ?: return
            if (Build.VERSION.SDK_INT >= 26) {
                focusRequest?.let { manager.abandonAudioFocusRequest(it) }
            } else {
                @Suppress("DEPRECATION")
                manager.abandonAudioFocus(focusListener)
            }
        } catch (_: Exception) {
            // Never fail stop for audio cleanup.
        }
    }
}
