package com.anpfuel.domain.profile

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ProfileBusinessFieldsRuleTest {
    @Test fun `business scope excludes canonical identity prices and unexpected services`() {
        assertFalse(ProfileBusinessFieldsRule.valid(mapOf("display_name" to "New")))
        assertFalse(ProfileBusinessFieldsRule.valid(mapOf("price" to "1")))
        assertFalse(ProfileBusinessFieldsRule.valid(mapOf("services" to "unknown")))
        assertTrue(ProfileBusinessFieldsRule.valid(mapOf("services" to "wifi,parking")))
    }
    @Test fun `UTF8 byte cap and description scalar boundary match server policy`() {
        assertFalse(ProfileBusinessFieldsRule.valid(mapOf("phone" to "é".repeat(129))))
        assertTrue(ProfileBusinessFieldsRule.valid(mapOf("description" to "😀".repeat(280))))
        assertFalse(ProfileBusinessFieldsRule.valid(mapOf("description" to "😀".repeat(281))))
        assertFalse(ProfileBusinessFieldsRule.valid(emptyMap()))
        assertFalse(ProfileBusinessFieldsRule.valid(mapOf("website" to " ")))
    }
}
