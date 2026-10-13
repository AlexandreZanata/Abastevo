package com.anpfuel.domain.repository
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
class TrainingConsentReviewTest {
 @Test fun `reading checkbox and confirmation are independent`() {
  val initial=TrainingConsentReview("owner","v1","a".repeat(64))
  assertFalse(initial.canConfirm);assertFalse(initial.check(true).canConfirm)
  val read=initial.finishReading();assertFalse(read.canConfirm)
  assertTrue(read.check(true).canConfirm);assertFalse(read.check(false).canConfirm)
  assertFalse(read.check(true).finishReading().canConfirm)
  assertFalse(TrainingConsentReview("","v1","a".repeat(64),true,true).canConfirm)
 }
}
