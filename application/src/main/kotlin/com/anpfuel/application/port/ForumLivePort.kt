package com.anpfuel.application.port

import com.anpfuel.domain.forum.ForumLiveEvent
import kotlinx.coroutines.flow.Flow

/**
 * Live events of one open conversation (B-BR-FOR-18). The flow completes when
 * the server ends the stream and fails on transport errors; the caller
 * decides when to reconnect.
 */
interface ForumLivePort {
    fun conversation(postId: String): Flow<ForumLiveEvent>
}
