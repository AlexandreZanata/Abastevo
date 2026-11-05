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
    }

    @Test
    fun `forum routes map to community tab`() {
        val uuid = UUID.randomUUID().toString()
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute(Routes.FORUM_FEED))
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute("forum/feed"))
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute(Routes.forumPost(uuid)))
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute("forum/posts/$uuid"))
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute("forum/posts/new"))
    }
}
