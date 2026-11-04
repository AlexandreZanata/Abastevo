package com.anpfuel.app.ui.map

import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFocusRequest
import android.media.AudioManager
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.speech.tts.TextToSpeech
import java.util.Locale
import java.util.UUID

/**
 * Local Android TTS only, owned explicitly during foreground guidance.
 * Accepts only an installed local voice that does not require network;
 * network/download-required voices refuse and guidance stays textual.
 * Uses navigation audio focus, stops on focus loss/calls and fences stale
 * engine callbacks by generation.
 */
class LocalGuidanceVoice internal constructor(
    private val appContext: Context,
    private val mainHandler: Handler,
    private val createSpeech: (TextToSpeech.OnInitListener, String?) -> TextToSpeech,
) {
    constructor(appContext: Context) : this(appContext, Handler(Looper.getMainLooper()), { listener, engine ->
        if (engine == null) TextToSpeech(appContext, listener) else TextToSpeech(appContext, listener, engine)
    })

    private var tts: TextToSpeech? = null
    private var initTimeout: Runnable? = null
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

    fun start(language: String, preferredEngine: String? = null, onAvailability: (Boolean) -> Unit) {
        stop()
        val target = ttsLocaleFor(language)
        if (target == null) {
            onAvailability(false)
            return
        }
        val token = generation
        val listener = TextToSpeech.OnInitListener { status ->
            // Android can call this listener before the constructor returns.
            // Queue completion so the instance exists and state stays on Main.
            mainHandler.post {
                if (token != generation || initTimeout == null) return@post
                val instance = tts ?: return@post
                cancelInitTimeout()
                if (status == TextToSpeech.SUCCESS && configureLocalVoice(instance, target)) {
                    available = true
                    onAvailability(true)
                } else {
                    stop()
                    onAvailability(false)
                }
            }
        }
        tts = try {
            createSpeech(listener, preferredEngine?.takeIf { it.isNotEmpty() })
        } catch (_: Exception) {
            onAvailability(false)
            return
        }
        val timeout = Runnable {
            if (token == generation) {
                stop()
                onAvailability(false)
            }
        }
        initTimeout = timeout
        mainHandler.postDelayed(timeout, VOICE_INIT_TIMEOUT_MILLIS)
    }

    private fun configureLocalVoice(instance: TextToSpeech, target: Locale): Boolean = try {
        val candidates = instance.voices.orEmpty().filter { voice ->
            !voice.isNetworkConnectionRequired &&
                TextToSpeech.Engine.KEY_FEATURE_NOT_INSTALLED !in voice.features.orEmpty() &&
                voice.locale.language == target.language &&
                (voice.locale.country.isEmpty() || target.country.isEmpty() || voice.locale.country == target.country)
        }
        val selected = candidates.firstOrNull { it.locale == target } ?: candidates.firstOrNull()
        if (selected == null || instance.setVoice(selected) != TextToSpeech.SUCCESS) {
            false
        } else {
            instance.setSpeechRate(1.0f)
            true
        }
    } catch (_: Exception) {
        false
    }

    private fun cancelInitTimeout() {
        initTimeout?.let(mainHandler::removeCallbacks)
        initTimeout = null
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
        cancelInitTimeout()
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

    private companion object {
        const val VOICE_INIT_TIMEOUT_MILLIS = 8_000L
    }
}
