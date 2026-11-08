package com.anpfuel.app.navigation

import java.util.UUID
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class ForumNavigationTest {

    @Test
    fun `forum routes and uuid validation`() {
        val uuid = UUID.randomUUID().toString()
        assertEquals("forum/feed", Routes.FORUM_FEED)
        assertEquals("forum/posts/{postId}", Routes.FORUM_POST)
        assertEquals("forum/posts/$uuid", Routes.forumPost(uuid))

        assertThrows(IllegalArgumentException::class.java) {
            Routes.forumPost("invalid-uuid")
        }
        assertThrows(IllegalArgumentException::class.java) {
            Routes.forumPost("")
        }

        assertEquals("forum/topics/new", Routes.FORUM_NEW_TOPIC)
        assertEquals("forum/topics/{topicId}/edit", Routes.FORUM_EDIT_TOPIC)
        assertEquals("forum/topics/$uuid/edit", Routes.forumEditTopic(uuid))

        assertThrows(IllegalArgumentException::class.java) {
            Routes.forumEditTopic("invalid-uuid")
        }
        assertThrows(IllegalArgumentException::class.java) {
            Routes.forumEditTopic("")
        }
    }

    @Test
    fun `forum routes accept uppercase ids and never crash on malformed ones`() {
        val upper = "3F2504E0-4F89-41D3-9A0C-0305E82C3301"
        assertEquals("forum/posts/${upper.lowercase()}", Routes.forumPost(upper))
        assertEquals("forum/topics/${upper.lowercase()}/edit", Routes.forumEditTopic(upper))
        assertEquals("forum/posts/${upper.lowercase()}", Routes.forumPostOrNull(upper))
        assertEquals(null, Routes.forumPostOrNull("1-1-1-1-1"))
        assertEquals(null, Routes.forumEditTopicOrNull("not-a-uuid"))
    }

    @Test
    fun `forum routes map to community tab`() {
        val uuid = UUID.randomUUID().toString()
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute(Routes.FORUM_FEED))
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute("forum/feed"))
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute(Routes.forumPost(uuid)))
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute("forum/posts/$uuid"))
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute("forum/posts/new"))
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute(Routes.FORUM_NEW_TOPIC))
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute(Routes.forumEditTopic(uuid)))
    }
}
