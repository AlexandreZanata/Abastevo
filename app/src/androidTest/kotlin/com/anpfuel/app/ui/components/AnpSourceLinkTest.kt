package com.anpfuel.app.ui.components

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.platform.UriHandler
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class AnpSourceLinkTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun opensTheOfficialSurveySource() {
        var opened: String? = null
        val uriHandler = object : UriHandler {
            override fun openUri(uri: String) {
                opened = uri
            }
        }
        compose.setContent {
            CompositionLocalProvider(LocalUriHandler provides uriHandler) {
                AnpSourceLink()
            }
        }

        compose.onNodeWithTag("anp_source_link").performClick()
        compose.runOnIdle {
            val context = InstrumentationRegistry.getInstrumentation().targetContext
            assertEquals(context.getString(R.string.anp_official_url), opened)
        }
    }
}
