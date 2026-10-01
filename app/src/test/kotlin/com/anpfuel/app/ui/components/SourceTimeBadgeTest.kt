package com.anpfuel.app.ui.components

import com.anpfuel.app.R
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class SourceTimeBadgeTest {

    @Test
    fun communityMapsToCommunityLabel() {
        assertEquals(R.string.source_badge_community, SourceTimeBadgeLabels.labelRes(PriceSourceKind.COMMUNITY))
        assertEquals(R.string.a11y_source_badge_community, SourceTimeBadgeLabels.a11yRes(PriceSourceKind.COMMUNITY))
    }

    @Test
    fun anpDatedMapsToAnpLabel() {
        assertEquals(R.string.source_badge_anp_dated, SourceTimeBadgeLabels.labelRes(PriceSourceKind.ANP_DATED))
        assertEquals(R.string.a11y_source_badge_anp_dated, SourceTimeBadgeLabels.a11yRes(PriceSourceKind.ANP_DATED))
    }

    @Test
    fun unknownMapsToUnknownLabel() {
        assertEquals(R.string.source_badge_unknown, SourceTimeBadgeLabels.labelRes(PriceSourceKind.UNKNOWN))
        assertEquals(R.string.a11y_source_badge_unknown, SourceTimeBadgeLabels.a11yRes(PriceSourceKind.UNKNOWN))
    }
}
