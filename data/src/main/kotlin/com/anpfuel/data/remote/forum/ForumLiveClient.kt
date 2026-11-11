package com.anpfuel.data.remote.forum

import com.anpfuel.application.port.ForumLivePort
import com.anpfuel.data.mapper.ForumMapper
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumLiveEvent
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.launch
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request

/** Longest accepted SSE line; one public comment event is far below it. */
private const val MAX_LINE_BYTES = 64L * 1024L

/**
 * Splits a server-sent event stream into (event, data) pairs. Comment lines
 * (": ping") and retry hints are skipped; multi-line data is joined with "\n".
 */
class ForumSseParser {
    private var name: String? = null
    private val data = StringBuilder()

    /** Feeds one line without its terminator; returns an event on a blank line. */
    fun feed(line: String): Pair<String, String>? {
        when {
            line.isEmpty() -> {
                val event = name ?: if (data.isNotEmpty()) "message" else null
                val payload = data.toString()
                name = null
                data.clear()
                return event?.let { it to payload }
            }
            line.startsWith(":") -> Unit
            line.startsWith("event:") -> name = line.removePrefix("event:").trimStart()
            line.startsWith("data:") -> {
                if (data.isNotEmpty()) data.append('\n')
                data.append(line.removePrefix("data:").trimStart())
            }
        }
        return null
    }
}

/**
 * Follows GET /v1/forum/posts/{id}/stream (B-BR-FOR-18). Anonymous and public:
 * the server sends only committed public projections. The flow completes when
 * the server ends the stream and fails with a ForumException otherwise.
 */
class ForumLiveClient(
    http: OkHttpClient,
    baseUrl: String,
) : ForumLivePort {

    private val origin = baseUrl.trimEnd('/').toHttpUrl()
    private val client = http.newBuilder()
        .followRedirects(false)
        .followSslRedirects(false)
        .retryOnConnectionFailure(false)
        .callTimeout(0, TimeUnit.MILLISECONDS)
        // The server pings every 20 s; a silent minute means the stream is gone.
        .readTimeout(60, TimeUnit.SECONDS)
        .build()

    override fun conversation(postId: String): Flow<ForumLiveEvent> = callbackFlow {
        val url = origin.newBuilder().addPathSegments("v1/forum/posts/$postId/stream").build()
        val call = client.newCall(
            Request.Builder()
                .url(url)
                .header("Accept", "text/event-stream")
                .header("Cache-Control", "no-store")
                .build(),
        )
        val reader = launch(Dispatchers.IO) {
            try {
                call.execute().use { response ->
                    if (!response.isSuccessful) {
                        close(
                            when (response.code) {
                                404 -> ForumException.NotFound("forum.not-found")
                                503 -> ForumException.Unavailable("forum.stream-busy")
                                else -> ForumException.Unavailable("forum.stream-failed")
                            },
                        )
                        return@use
                    }
                    val source = response.body?.source()
                    if (source == null) {
                        close(ForumException.Invalid("forum.bad-response"))
                        return@use
                    }
                    val parser = ForumSseParser()
                    while (!source.exhausted()) {
                        val line = source.readUtf8LineStrict(MAX_LINE_BYTES)
                        val frame = parser.feed(line) ?: continue
                        val event = ForumMapper.liveEvent(frame.first, frame.second) ?: continue
                        send(event)
                        if (event == ForumLiveEvent.Reconnect || event == ForumLiveEvent.Resync || event == ForumLiveEvent.PostGone) {
                            break
                        }
                    }
                    close()
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: IOException) {
                close(ForumException.Offline("forum.stream-offline"))
            } catch (e: ForumException) {
                close(e)
            } catch (e: Exception) {
                close(ForumException.Invalid("forum.bad-response"))
            }
        }
        awaitClose {
            call.cancel()
            reader.cancel()
        }
    }
}
