package com.anpfuel.app

import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.preferences.TermsAcceptancePrefsStore
import com.anpfuel.domain.legal.PublishedLegalDocuments
import com.anpfuel.domain.legal.TermsAcceptanceRule
import org.junit.rules.TestWatcher
import org.junit.runner.Description

/**
 * MainActivity journeys start behind the first-use terms gate (B-BR-TA01).
 * Declare this with a lower order than the activity rule so the record exists
 * before the activity is created; the gate itself is covered by TermsGateDeviceTest.
 */
class AcceptedTermsRule : TestWatcher() {
    override fun starting(description: Description) {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val record = TermsAcceptanceRule.accept(PublishedLegalDocuments.CURRENT, ticked = true, nowMillis = System.currentTimeMillis())
        check(TermsAcceptancePrefsStore(context).save(record)) { "terms acceptance could not be seeded" }
    }
}
